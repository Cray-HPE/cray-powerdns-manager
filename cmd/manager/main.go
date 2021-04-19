package main

import (
	"crypto/tls"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/namsral/flag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"stash.us.cray.com/CASMNET/cray-powerdns-manager/internal/httpLogger"
	"strings"
	"time"
)

var (
	router *gin.Engine

	httpClient *retryablehttp.Client

	atomicLevel zap.AtomicLevel
	logger      *zap.Logger
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

	// Setup core components.
	setupLogging()
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
}
