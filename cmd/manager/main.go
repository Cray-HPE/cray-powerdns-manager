package main

import (
	"context"
	"crypto/tls"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/joeig/go-powerdns"
	"github.com/namsral/flag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"os/signal"
	"stash.us.cray.com/CASMNET/cray-powerdns-manager/internal/httpLogger"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	baseDomain = flag.String("base_domain", "shasta.dev.cray.com",
		"Base master domain from which to build all other records on top of")
	slsURL = flag.String("sls_url", "http://cray-sls",
		"System Layout Service URL")
	hsmURL = flag.String("hsm_url", "http://cray-smd",
		"State Manager URL")

	pdnsURL = flag.String("pdns_url", "http://localhost:9090",
		"PowerDNS URL")
	pdnsAPIKey = flag.String("pdns_api_key", "cray",
		"PowerDNS API Key")

	trueUpSleepInterval = flag.Int("true_up_sleep_interval", 30,
		"Time to sleep between true up runs")

	router *gin.Engine

	pdns *powerdns.Client

	httpClient *retryablehttp.Client

	atomicLevel zap.AtomicLevel
	logger      *zap.Logger

	Running   = true
	WaitGroup sync.WaitGroup
	ctx       context.Context

	APIServer *http.Server = nil

	trueUpShutdown chan bool
	trueUpRunNow chan bool
	trueUpInProgress bool
	trueUpMtx sync.Mutex
)

func setupLogging() {
	logLevel := os.Getenv("LOG_LEVEL")
	logLevel = strings.ToUpper(logLevel)

	atomicLevel = zap.NewAtomicLevel()

	encoderCfg := zap.NewProductionEncoderConfig()
	logger = zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		atomicLevel,
	))

	switch logLevel {
	case "DEBUG":
		atomicLevel.SetLevel(zap.DebugLevel)
		gin.SetMode(gin.DebugMode)
	case "INFO":
		atomicLevel.SetLevel(zap.InfoLevel)
		gin.SetMode(gin.ReleaseMode)
	case "WARN":
		atomicLevel.SetLevel(zap.WarnLevel)
		gin.SetMode(gin.ReleaseMode)
	case "ERROR":
		atomicLevel.SetLevel(zap.ErrorLevel)
		gin.SetMode(gin.ReleaseMode)
	case "FATAL":
		atomicLevel.SetLevel(zap.FatalLevel)
		gin.SetMode(gin.ReleaseMode)
	case "PANIC":
		atomicLevel.SetLevel(zap.PanicLevel)
		gin.SetMode(gin.ReleaseMode)
	default:
		atomicLevel.SetLevel(zap.InfoLevel)
		gin.SetMode(gin.ReleaseMode)
	}
}

func main() {
	// Parse the arguments.
	flag.Parse()

	// Setup logging.
	setupLogging()

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	trueUpShutdown = make(chan bool)
	trueUpRunNow = make(chan bool)

	go func() {
		<-c

		logger.Info("Shutting down...")

		Running = false

		// Cancel the context to cancel any in progress HTTP requests.
		cancel()

		trueUpShutdown <- true

		if APIServer != nil {
			serverCtx, serverCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer serverCancel()
			if err := APIServer.Shutdown(serverCtx); err != nil {
				logger.Panic("API server forced to shutdown!", zap.Error(err))
			}
		}
	}()

	// Add to the wait group so we spin on it later.
	WaitGroup.Add(1)
	logger.Info("Starting API server.")
	setupAPI()

	// For performance reasons we'll keep the client that was created for this base request and reuse it later.
	httpClient = retryablehttp.NewClient()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient.HTTPClient.Transport = transport

	httpClient.RetryMax = 2
	httpClient.RetryWaitMax = time.Second * 2

	// Also, since we're using Zap logger it make sense to set the logger to use the one we've already setup.
	newHttpLogger := httpLogger.NewHTTPLogger(logger)
	httpClient.Logger = newHttpLogger

	// Setup the PowerDNS configuration.
	pdns = powerdns.NewClient(*pdnsURL, "localhost", map[string]string{"X-API-Key": *pdnsAPIKey}, nil)

	// Kick off the true up loop.
	WaitGroup.Add(1)
	logger.Info("Starting true up loop.")
	go trueUpDNS()


	// We'll spend pretty much the rest of life blocking on the next line.
	WaitGroup.Wait()
}
