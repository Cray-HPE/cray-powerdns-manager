package main

import (
	"fmt"
	"github.com/joeig/go-powerdns/v2"
	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
	"net/http"
	"reflect"
	sls_common "stash.us.cray.com/HMS/hms-sls/pkg/sls-common"
	"strings"
	"time"
)

type nameserver struct {
	FQDN string
	IP   string
}

func makeDomainCanonical(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	} else {
		return fmt.Sprintf("%s.", domain)
	}
}

func trueUpMasterZone(baseDomain string, nameservers []nameserver) (masterZone *powerdns.Zone, err error) {
	// First and foremost, check to make sure there is a master zone for the base domain.
	masterZone, err = pdns.Zones.Get(baseDomain)
	if err != nil {
		pdnsErr, ok := err.(*powerdns.Error)
		if !ok {
			logger.Error("Error not a PowerDNS error type!", zap.Error(err))
			return
		} else {
			if pdnsErr.StatusCode == http.StatusNotFound {
				logger.Info("Base domain not found, adding.")

				var nameserverFQDNs []string
				var rrSets []powerdns.RRset

				// We have to create A recoreds for all the name servers otherwise it won't let us create the zone.
				for _, nameserver := range nameservers {
					canonicalName := makeDomainCanonical(nameserver.FQDN)
					nameserverFQDNs = append(nameserverFQDNs, canonicalName)

					nameserverRRSet := powerdns.RRset{
						Name: &canonicalName,
						Type: powerdns.RRTypePtr(powerdns.RRTypeA),
						TTL:  powerdns.Uint32(3600),
						Records: []powerdns.Record{
							{
								Content:  powerdns.String(nameserver.IP),
								Disabled: powerdns.Bool(false),
							},
						},
					}

					rrSets = append(rrSets, nameserverRRSet)
				}

				zone := &powerdns.Zone{
					Name:        &baseDomain,
					Kind:        powerdns.ZoneKindPtr(powerdns.MasterZoneKind),
					DNSsec:      powerdns.Bool(true),
					Nameservers: nameserverFQDNs,
					RRsets:      rrSets,
				}

				masterZone, err = pdns.Zones.Add(zone)
				if err != nil {
					logger.Error("Failed to add master zone!",
						zap.Error(err), zap.Any("masterZone", *masterZone))
					return
				}

				return
			} else {
				logger.Error("Got unexpected status code from attempt to get zone!", zap.Error(err))
				return
			}
		}
	}

	logger.Debug("Master zone already exists.")

	return
}

func buildStaticRRSets(networks []sls_common.Network) (staticRRSets []powerdns.RRset, err error) {
	for _, network := range networks {
		networkDomain := strings.ToLower(network.Name)

		var networkProperties NetworkExtraProperties
		err = mapstructure.Decode(network.ExtraPropertiesRaw, &networkProperties)
		if err != nil {
			return
		}

		for _, subnet := range networkProperties.Subnets {
			for _, reservation := range subnet.IPReservations {
				// Avoid bad names.
				if strings.Contains(reservation.Name, ".") {
					continue
				}

				primaryName := fmt.Sprintf("%s.%s.%s.", reservation.Name, networkDomain, *baseDomain)
				// Create the primary A record.
				primaryRRset := powerdns.RRset{
					Name:       powerdns.String(primaryName),
					Type:       powerdns.RRTypePtr(powerdns.RRTypeA),
					TTL:        powerdns.Uint32(3600),
					ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
					Records: []powerdns.Record{
						{
							Content:  powerdns.String(string(reservation.IPAddress)),
							Disabled: powerdns.Bool(false),
						},
					},
				}
				staticRRSets = append(staticRRSets, primaryRRset)

				// Now create CNAME records for each of the aliases.
				for _, alias := range reservation.Aliases {
					// Avoid bad names.
					if strings.Contains(alias, ".") {
						continue
					}

					aliasRRset := powerdns.RRset{
						Name:       powerdns.String(fmt.Sprintf("%s.%s.%s.", alias, networkDomain, *baseDomain)),
						Type:       powerdns.RRTypePtr(powerdns.RRTypeCNAME),
						TTL:        powerdns.Uint32(3600),
						ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
						Records: []powerdns.Record{
							{
								Content:  powerdns.String(primaryName),
								Disabled: powerdns.Bool(false),
							},
						},
					}
					staticRRSets = append(staticRRSets, aliasRRset)
				}
			}
		}
	}

	return
}

func rrsetsEqual(a powerdns.RRset, b powerdns.RRset) bool {
	if *a.Name != *b.Name ||
		!reflect.DeepEqual(a.Records, b.Records) ||
		*a.TTL != *b.TTL ||
		*a.Type != *b.Type {
		return false
	}

	return true
}

// trueUpRRSets verifies all of the RRsets for the zone are as they should be.
// There are a total of 3 possibilities for each RRset:
//	1) The RRset doesn't exist at all.
//	2) The RRset exists but the records are not correct.
//  3) The RRset exists and shouldn't.
func trueUpRRSets(rrsets []powerdns.RRset, zone *powerdns.Zone) (didSomething bool) {
	// To make this process a lot quicker first build up a map of names to RRsets for O(1) lookups later.
	zoneRRsetMap := make(map[string]powerdns.RRset)
	desiredRRSetMap := make(map[string]powerdns.RRset)

	for _, zoneRRset := range zone.RRsets {
		zoneRRsetMap[*zoneRRset.Name] = zoneRRset
	}
	for _, desiredRRset := range rrsets {
		desiredRRSetMap[*desiredRRset.Name] = desiredRRset
	}

	for _, desiredRRset := range desiredRRSetMap {
		zoneRRset, found := zoneRRsetMap[*desiredRRset.Name]

		patchLogger := logger.With(zap.Any("desiredRRset", desiredRRset),
			zap.Any("zoneRRset", zoneRRset))

		if found {
			// Case 2 - is the RRSet correct?
			if !rrsetsEqual(desiredRRset, zoneRRset) {
				err := pdns.Records.Patch(*zone.Name, &powerdns.RRsets{Sets: []powerdns.RRset{desiredRRset}})
				if err != nil {
					patchLogger.Error("Failed to patch RRset!", zap.Error(err))
				} else {
					patchLogger.Info("Patched RRset")
					didSomething = true
				}
			} else {
				logger.Debug("RRset already at desired config", zap.Any("zoneRRset", zoneRRset))
			}
		} else {
			// Case 1 - not found, add it.
			err := pdns.Records.Patch(*zone.Name, &powerdns.RRsets{Sets: []powerdns.RRset{desiredRRset}})
			if err != nil {
				patchLogger.Error("Failed to add RRset!", zap.Error(err))
			} else {
				patchLogger.Info("Added RRset")
				didSomething = true
			}
		}
	}

	// Case 3 - should this exist?
	for _, zoneRRset := range zoneRRsetMap {
		_, found := desiredRRSetMap[*zoneRRset.Name]

		if !found &&
			zoneRRset.Type == powerdns.RRTypePtr(powerdns.RRTypeNS) {
			deleteLogger := logger.With(zap.Any("zoneRRset", zoneRRset))

			err := pdns.Records.Delete(*zone.Name, *zoneRRset.Name, *zoneRRset.Type)
			if err != nil {
				deleteLogger.Error("Failed to delete RRset!", zap.Error(err))
			} else {
				deleteLogger.Info("Deleted RRset")
				didSomething = true
			}
		}
	}

	return
}

func buildNameserverRRset(nameserver nameserver) powerdns.RRset {
	return powerdns.RRset{
		Name:       powerdns.String(makeDomainCanonical(nameserver.FQDN)),
		Type:       powerdns.RRTypePtr(powerdns.RRTypeA),
		TTL:        powerdns.Uint32(3600),
		ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
		Records: []powerdns.Record{
			{
				Content:  powerdns.String(makeDomainCanonical(nameserver.FQDN)),
				Disabled: powerdns.Bool(false),
			},
		},
	}
}

func trueUpDNS() {
	logger.Info("Running true up loop at interval.", zap.Int("trueUpLoopInterval", *trueUpSleepInterval))

	defer WaitGroup.Done()

	for Running {
		// This block is at the very top of this loop so that we can `continue` our way to the next iteration if there
		// is an error and not just blow past the sleep block.
		select {
		case <-trueUpShutdown:
			return
		case <-trueUpRunNow: // For those impatient type.
		case <-time.After(time.Duration(*trueUpSleepInterval) * time.Second):
			logger.Debug("Running true up loop.")
		}

		trueUpMtx.Lock()
		trueUpInProgress = true
		trueUpMtx.Unlock()

		var nameserverRRsets []powerdns.RRset

		masterNameserverSplit := strings.Split(*masterServer, "/")
		if len(masterNameserverSplit) != 2 {
			logger.Fatal("Master nameserver does not have FQDN/IP format!",
				zap.String("masterServer", *masterServer))
		}
		masterNameserver := nameserver{
			FQDN: masterNameserverSplit[0],
			IP:   masterNameserverSplit[1],
		}
		nameserverRRsets = append(nameserverRRsets, buildNameserverRRset(masterNameserver))

		nameServers := []nameserver{masterNameserver}
		for _, slaveServer := range strings.Split(*slaveServers, ",") {
			nameserverSplit := strings.Split(slaveServer, "/")
			if len(nameserverSplit) != 2 {
				logger.Fatal("Slave nameserver does not have FQDN/IP format!",
					zap.String("slaveServer", slaveServer))
			}
			slaveNameserver := nameserver{
				FQDN: nameserverSplit[0],
				IP:   nameserverSplit[1],
			}

			nameServers = append(nameServers, slaveNameserver)
			nameserverRRsets = append(nameserverRRsets, buildNameserverRRset(slaveNameserver))
		}

		masterZone, err := trueUpMasterZone(*baseDomain, nameServers)
		if err != nil {
			continue
		}

		networks, err := getSLSNetworks()
		if err != nil {
			logger.Error("Failed to get networks from SLS!", zap.Error(err))
			continue
		}

		staticRRSets, err := buildStaticRRSets(networks)
		if err != nil {
			logger.Error("Failed to build static RRsets!", zap.Error(err))
		}

		// At this point we have computed every correct RRSet necessary. Now the only task is to add the ones that are
		// missing and remove the ones that shouldn't be there.
		var finalRRSet []powerdns.RRset
		finalRRSet = append(finalRRSet, staticRRSets...)

		didSomething := trueUpRRSets(finalRRSet, masterZone)

		// Force a sync to any slave servers if we did something.
		if didSomething {
			result, err := pdns.Zones.Notify(*baseDomain)
			if err != nil {
				logger.Error("Failed to notify slave server(s)!", zap.Error(err))
			} else {
				logger.Info("Notified slave server(s)", zap.Any("result", result))
			}
		}

		trueUpMtx.Lock()
		trueUpInProgress = false
		trueUpMtx.Unlock()
	}

	logger.Info("True up loop shutdown.")
}
