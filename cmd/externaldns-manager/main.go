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
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Cray-HPE/cray-powerdns-manager/internal/common"
	"github.com/Cray-HPE/cray-powerdns-manager/internal/httpLogger"
	"github.com/gin-gonic/gin"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/joeig/go-powerdns/v2"
	"github.com/namsral/flag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	pdnsURL             = flag.String("pdns_url", "http://localhost:9090", "PowerDNS URL")
	pdnsAPIKey          = flag.String("pdns_api_key", "cray", "PowerDNS API Key")
	trueUpSleepInterval = flag.Int("true_up_sleep_interval", 30, "Time to sleep between true up runs")

	pdns *powerdns.Client

	httpClient *retryablehttp.Client

	trueUpShutdown   chan bool
	trueUpRunNow     chan bool
	trueUpInProgress bool
	trueUpMtx        sync.Mutex
	//ctx              context.Context

	WaitGroup sync.WaitGroup

	router    *gin.Engine
	APIServer *http.Server = nil

	atomicLevel zap.AtomicLevel
	logger      *zap.Logger

	Running = true
)

// ProcessExternalDNSRecord
// When a TXT record with an externaldns signature is found locate the corresponding A record
// and generate a PTR record and a TXT record with an externaldns-manager signature, so we can
// keep track of it.
func ProcessExternalDNSRecord(rrSet powerdns.RRset, zone *powerdns.Zone, allZones []*powerdns.Zone) (powerdns.RRset, powerdns.RRset, error) {
	var rrSetReverse, rrSetTXT powerdns.RRset
	var err error = nil

	logger.Debug("ProcessExternalDNSRecord: Found ExternalDNS TXT record", zap.Any("txt", *rrSet.Name), zap.Any("content", *rrSet.Records[0].Content))

	for _, name := range zone.RRsets {
		if *name.Type == powerdns.RRTypeA && *name.Name == *rrSet.Name {
			for _, record := range name.Records {
				logger.Debug("Found corresponding A record", zap.Any("name", *name.Name), zap.Any("ip", *record.Content))

				// Check to see if reverse zone exists before doing anything.
				var revZone *net.IPNet
				_, revZone, _ = net.ParseCIDR(fmt.Sprintf("%s/32", *record.Content))

				var found = false
				for _, zone := range allZones {
					if *zone.Name == common.MakeDomainCanonical(common.GetReverseZoneName(revZone)) {
						found = true
					}
				}
				if !found {
					logger.Error("Calculated reverse zone does not match any existing zone", zap.Any("reverseZoneName", common.GetReverseZoneName(revZone)))
					err = fmt.Errorf("cannot find reverse zone")
					break
				}

				logger.Debug("Generating PTR record", zap.Any("ipaddr", common.GetReverseName(strings.Split(*record.Content, "."))))
				rrSetReverse = powerdns.RRset{
					Name:       powerdns.String(common.MakeDomainCanonical(common.GetReverseName(strings.Split(*record.Content, ".")))),
					Type:       powerdns.RRTypePtr(powerdns.RRTypePTR),
					TTL:        powerdns.Uint32(3600),
					ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
					Records: []powerdns.Record{
						{
							Content:  powerdns.String(*name.Name),
							Disabled: powerdns.Bool(false),
						},
					},
				}

				// Build an associated TXT record with an externaldns-manager signature, so
				// it can later be determined whether it's stale and needs deleting
				rrSetTXT = powerdns.RRset{
					Name:       powerdns.String(common.MakeDomainCanonical(common.GetReverseName(strings.Split(*record.Content, ".")))),
					Type:       powerdns.RRTypePtr(powerdns.RRTypeTXT),
					TTL:        powerdns.Uint32(3600),
					ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
					Records: []powerdns.Record{
						{
							// Ugly but TXT records have to start with a quote
							Content:  powerdns.String("\"externaldns-manager/" + *name.Name + "\""),
							Disabled: powerdns.Bool(false),
						},
					},
				}
			}
		}
	}
	return rrSetReverse, rrSetTXT, err
}

// ProcessManagerRecord
// When a TXT record with an externaldns-manager signature is found determine the associated
// PTR record and then look for an A record. If one exists do nothing, otherwise schedule the
// PTR and TXT record for deletion.
func ProcessManagerRecord(rrSet powerdns.RRset, zone *powerdns.Zone, allZones []*powerdns.Zone) ([]powerdns.RRset, error) {
	var recordsToDelete []powerdns.RRset
	var foundPTRRecord = false
	var ptrRecord powerdns.RRset

	logger.Debug("ProcessManagerRecord: Found Manager TXT record", zap.Any("name", *rrSet.Name),
		zap.Any("content", *rrSet.Records[0].Content))

	for _, record := range zone.RRsets {
		if *rrSet.Name == *record.Name && *record.Type == powerdns.RRTypePTR {
			logger.Debug("ProcessManagerRecord: Found associated PTR record", zap.Any("name", *record.Name),
				zap.Any("content", *record.Records[0].Content))
			foundPTRRecord = true
			ptrRecord = record
		}
	}
	// We should never end up in the scenario where an externaldns-manager TXT record exists without a PTR record.
	if !foundPTRRecord {
		logger.Error("ProcessManagerRecord: Cannot find PTR record for TXT record", zap.Any("txt", *rrSet.Name))
		err := fmt.Errorf("cannot find PTR record")
		rrSet.ChangeType = powerdns.ChangeTypePtr(powerdns.ChangeTypeDelete)
		recordsToDelete = append(recordsToDelete, rrSet)
		return recordsToDelete, err
	}

	// Need to figure out the associated A record
	// TODO: Improve efficiency, it would be better to figure out which zone the A record would reside in then only scan that.
	myIp := common.GetForwardIP(*ptrRecord.Name)
	var foundARecord = false
	for _, z := range allZones {
		for _, rrSet := range z.RRsets {
			if *rrSet.Records[0].Content == myIp && strings.HasSuffix(*ptrRecord.Records[0].Content, *rrSet.Name) {
				logger.Debug("ProcessManagerRecord: Found A Record", zap.Any("rrSet", rrSet))
				foundARecord = true
			}
		}
	}

	if foundARecord {
		logger.Debug("ProcessManagerRecord: Found A record, nothing to do")
	} else {
		logger.Info("ProcessManagerRecord: Stale records scheduled for deletion",
			zap.Any("txt", rrSet), zap.Any("ptr", ptrRecord))
		ptrRecord.ChangeType = powerdns.ChangeTypePtr(powerdns.ChangeTypeDelete)
		rrSet.ChangeType = powerdns.ChangeTypePtr(powerdns.ChangeTypeDelete)
		recordsToDelete = append(recordsToDelete, ptrRecord)
		recordsToDelete = append(recordsToDelete, rrSet)
	}

	return recordsToDelete, nil
}

/* There are three cases to consider here
   1) An externaldns TXT record but has no corresponding PTR record - Create it
   2) A PTR record exists with a cray-powerdns TXT record but no corresponding externaldns TXT record - Delete it
   3) Both A and PTR records exist but the IPs do not match - Update it - The externaldns created record is *always* authoritative
*/
func doLoop() {

	defer WaitGroup.Done()

	for Running {

		select {
		case <-trueUpShutdown:
			return
		case <-trueUpRunNow: // do it
		case <-time.After(time.Duration(*trueUpSleepInterval) * time.Second):
			logger.Debug("Running loop")
		}

		trueUpMtx.Lock()
		trueUpInProgress = true
		trueUpMtx.Unlock()

		// do stuff here
		logger.Info("Processing records...")

		// Get all the Zones.
		zones, err := pdns.Zones.List()
		if err != nil {
			panic(err)
		}

		// create list of Zones
		var allZones []*powerdns.Zone
		for _, zone := range zones {
			zone, err := pdns.Zones.Get(*zone.Name)
			if err != nil {
				panic(err)
			}
			allZones = append(allZones, zone)
		}

		// Map of zone -> rrSet which will be used when building the final patch request.
		actionableRRSetMap := make(map[string]*powerdns.RRsets)

		for _, zone := range zones {
			actionableRRSetMap[*zone.Name] = &powerdns.RRsets{Sets: []powerdns.RRset{}}
		}

		// slice of rrSets that should be considered for patching
		var patchRRSets []powerdns.RRset
		for _, zone := range allZones {

			logger.Debug("Processing zone", zap.Any("zone", *zone.Name))
			for _, rrSet := range zone.RRsets {

				if *rrSet.Type == powerdns.RRTypeTXT {

					// Check to see whether a TXT record was created by externaldns, externaldns-manager or
					// is a record we should just leave alone.
					if strings.Contains(*rrSet.Records[0].Content, "heritage=external-dns") {
						rrSetReverse, rrSetTXT, err := ProcessExternalDNSRecord(rrSet, zone, allZones)
						if err != nil {
							logger.Error("Could not determine reverse zone for record", zap.String("name", *rrSet.Name))
							break
						}

						// The PowerDNS API will not accept a bulk submission containing duplicate records and there
						// are numerous services bound to the LoadBalancers (for example
						// cray-oauth2-proxies-customer-management-ingress) that all point to the same IP address.
						var addMe = true
						for _, zone := range patchRRSets {
							if *zone.Name == *rrSetReverse.Name {
								logger.Debug("Refusing to add duplicate", zap.Any("rrSetReverse", rrSetReverse))
								addMe = false
								break
							}
						}
						if addMe == true {
							logger.Debug("Adding entry", zap.Any("rrSetReverse", rrSetReverse))
							patchRRSets = append(patchRRSets, rrSetReverse)
							patchRRSets = append(patchRRSets, rrSetTXT)
						}

					} else if strings.Contains(*rrSet.Records[0].Content, "externaldns-manager") {
						recordsToDelete, err := ProcessManagerRecord(rrSet, zone, allZones)
						if err != nil {
							logger.Error("Deleting orphaned Manager TXT record", zap.Any("record", recordsToDelete))
						}
						if len(recordsToDelete) > 0 {
							logger.Debug("Deleting records", zap.Any("recordsToDelete", recordsToDelete))
							patchRRSets = append(patchRRSets, recordsToDelete...)
						}
					} else {
						logger.Debug("Found TXT record without ExternalDNS or Manager signature, ignoring.",
							zap.Any("name", *rrSet.Name), zap.Any("content", *rrSet.Records[0].Content))
						continue
					}
				}
			}
		}

		// TODO: Figure out if a record already exists and remove it from the patch list to reduce the size of the API call.
		// The PowerDNS API does not appear to increment the SOA serial unless anything actually changes so this approach
		// does not result in unnecessary AXFR notify requests being generated.
		for _, zone := range patchRRSets {
			zoneSets := &actionableRRSetMap[*common.GetZoneForRRSet(zone, allZones)].Sets
			*zoneSets = append(*zoneSets, zone)
		}

		logger.Debug("Patch list", zap.Any("actionableRRSetMap", actionableRRSetMap))

		for zone, rrSets := range actionableRRSetMap {
			zoneLogger := logger.With(zap.String("zone", zone))

			if len(rrSets.Sets) > 0 {
				// Do all the patching (which is additions, changes, and deletes) in one API call...pretty cool.
				err := pdns.Records.Patch(zone, rrSets)
				if err != nil {
					zoneLogger.Error("Failed to patch RRSets!", zap.Error(err), zap.Any("zone", zone))
				} else {
					zoneLogger.Info("Patched RRSets")
				}
			}
		}

		trueUpMtx.Lock()
		trueUpInProgress = false
		trueUpMtx.Unlock()
	}

	logger.Info("Loop shutting down...")

}

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

	// Get command line arguments
	flag.Parse()

	// Setup logging
	setupLogging()

	// var cancel context.CancelFunc
	// ctx, cancel = context.WithCancel(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	trueUpShutdown = make(chan bool)
	trueUpRunNow = make(chan bool, 1)

	go func() {
		<-c

		logger.Info("Shutting down...")

		Running = false

		//cancel()

		trueUpShutdown <- true

		if APIServer != nil {
			serverCtx, serverCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer serverCancel()
			if err := APIServer.Shutdown(serverCtx); err != nil {
				logger.Panic("API server forced to shutdown!", zap.Error(err))
			}
		}

	}()

	WaitGroup.Add(1)
	logger.Info("Starting API server.")
	setupAPI()

	// For performance reasons we'll keep the client that was created for this base request and reuse it later.
	httpClient = retryablehttp.NewClient()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient.HTTPClient.Transport = transport

	httpClient.RetryMax = 3
	httpClient.RetryWaitMax = time.Second * 2

	// Also, since we're using Zap logger it makes sense to set the logger to use the one we've already setup.
	newHttpLogger := httpLogger.NewHTTPLogger(logger)
	httpClient.Logger = newHttpLogger

	// Set up the PowerDNS configuration.
	pdns = powerdns.NewClient(*pdnsURL, "localhost", map[string]string{"X-API-Key": *pdnsAPIKey},
		httpClient.HTTPClient)

	WaitGroup.Add(1)
	logger.Info("Starting up main loop...")
	go doLoop()

	// Seed the fist run since we start the loop with the select block.
	trueUpRunNow <- true

	// We'll spend pretty much the rest of life blocking on the next line.
	WaitGroup.Wait()
}
