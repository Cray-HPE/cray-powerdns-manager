/*
 *
 *  MIT License
 *
 *  (C) Copyright 2022 Hewlett Packard Enterprise Development LP
 *
 *  Permission is hereby granted, free of charge, to any person obtaining a
 *  copy of this software and associated documentation files (the "Software"),
 *  to deal in the Software without restriction, including without limitation
 *  the rights to use, copy, modify, merge, publish, distribute, sublicense,
 *  and/or sell copies of the Software, and to permit persons to whom the
 *  Software is furnished to do so, subject to the following conditions:
 *
 *  The above copyright notice and this permission notice shall be included
 *  in all copies or substantial portions of the Software.
 *
 *  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 *  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 *  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 *  THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
 *  OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
 *  ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
 *  OTHER DEALINGS IN THE SOFTWARE.
 *
 */
package main

import (
	"context"
	"crypto/tls"
	"github.com/Cray-HPE/cray-powerdns-manager/internal/common"
	"github.com/Cray-HPE/cray-powerdns-manager/internal/httpLogger"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/joeig/go-powerdns/v2"
	"github.com/namsral/flag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	baseDomain = flag.String("base_domain", "shasta.dev.cray.com",
		"Base master domain from which to build all other records on top of")
	masterServer = flag.String("primary_server", "primary/192.168.53.4",
		"name/IP of this primary DNS server")
	slaveServers = flag.String("secondary_servers", "secondary/192.168.53.5",
		"Comma separated list of secondary DNS name/IPs")
	notifyZones = flag.String("notify_zones", "",
		"Comma separated list of zones for which a DNS NOTIFY should be sent to the slave servers")
	keyDirectory = flag.String("key_directory", "./keys",
		"Path to directory containing ICS formatted private DNSSEC keys for one or more zones")

	slsURL = flag.String("sls_url", "http://cray-sls", "System Layout Service URL")
	hsmURL = flag.String("hsm_url", "http://cray-smd", "State Manager URL")

	pdnsURL = flag.String("pdns_url", "http://localhost:9090",
		"PowerDNS URL")
	pdnsAPIKey = flag.String("pdns_api_key", "cray",
		"PowerDNS API Key")

	trueUpSleepInterval = flag.Int("true_up_sleep_interval", 30,
		"Time to sleep between true up runs")

	ignoreSLSNetworks = flag.String("sls_ignore", "BICAN",
		"Comma separated list of SLS networks to ignore, should always include BICAN")

	soaRefresh = flag.String("soa_refresh", "10800",
		"The number of seconds before the zone should be refreshed")
	soaRetry = flag.String("soa_retry", "3600",
		"The number of seconds before a failed refresh should be retried")
	soaExpiry = flag.String("soa_expiry", "604800",
		"The upper limit in seconds before a zone is considered no longer authoritative")
	soaMinimum = flag.String("soa_minimum", "3600",
		"The negative result TTL")

	router *gin.Engine

	pdns *powerdns.Client

	httpClient *retryablehttp.Client

	atomicLevel zap.AtomicLevel
	logger      *zap.Logger

	Running   = true
	WaitGroup sync.WaitGroup
	ctx       context.Context

	APIServer *http.Server = nil

	trueUpShutdown   chan bool
	trueUpRunNow     chan bool
	trueUpInProgress bool
	trueUpMtx        sync.Mutex

	token string

	notifyZonesArray       []string
	ignoreSLSNetworksArray []string
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

	token = os.Getenv("TOKEN")

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	trueUpShutdown = make(chan bool)
	trueUpRunNow = make(chan bool, 1)

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

	httpClient.RetryMax = 4
	httpClient.RetryWaitMax = time.Second * 2

	// Also, since we're using Zap logger it make sense to set the logger to use the one we've already setup.
	newHttpLogger := httpLogger.NewHTTPLogger(logger)
	httpClient.Logger = newHttpLogger

	// Setup the PowerDNS configuration.
	pdns = powerdns.NewClient(*pdnsURL, "localhost", map[string]string{"X-API-Key": *pdnsAPIKey},
		httpClient.StandardClient())

	// Parse any DNSSEC keys.
	err := ParseDNSKeys()
	if err != nil {
		logger.Error("Failed to parse DNSSEC keys directory!", zap.Error(err))
	} else {
		for _, key := range DNSKeys {
			if key.Type == common.TSIGKeyType {
				logger.Info("Parsed TSIG key", zap.Any("key", key))
			} else {
				logger.Info("Parsed DNSSEC key", zap.Any("key", key))
			}
		}
	}

	// If there are any TSIG keys, load them into PowerDNS.
	for _, key := range DNSKeys {
		if key.Type == common.TSIGKeyType {
			err := AddOrUpdateTSIGKey(key)
			if err != nil {
				logger.Error("Failed to add TSIG key!", zap.Error(err), zap.Any("key", key))
			}
		}
	}

	// Compute an array of the zones for which to notify.
	if *notifyZones != "" {
		notifyZonesArray = strings.Split(*notifyZones, ",")
	}
	if len(notifyZonesArray) == 0 {
		logger.Info("Sending DNS NOTIFY for all zones")
	} else {
		logger.Info("Sending DNS NOTIFY for zones", zap.Strings("notifyZonesArray", notifyZonesArray))
	}

	// Build a list of SLS networks to ignore.
	if *ignoreSLSNetworks != "" {
		ignoreSLSNetworksArray = strings.Split(*ignoreSLSNetworks, ",")
		logger.Debug("Excluding the following SLS networks from zone generation", zap.Strings("ignoreSLSNetworksArray", ignoreSLSNetworksArray))
	}

	// Kick off the true up loop.
	WaitGroup.Add(1)
	logger.Info("Starting true up loop.")
	go trueUpDNS()

	// Seed the fist run since we start the loop with the select block.
	trueUpRunNow <- true

	// We'll spend pretty much the rest of life blocking on the next line.
	WaitGroup.Wait()
}
