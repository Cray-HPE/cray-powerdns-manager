package main

import (
	"fmt"
	"github.com/joeig/go-powerdns/v2"
	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
	"net"
	"net/http"
	"reflect"
	sls_common "stash.us.cray.com/HMS/hms-sls/pkg/sls-common"
	"stash.us.cray.com/HMS/hms-smd/pkg/sm"
	"strings"
	"time"
)

const rdnsDomain = ".in-addr.arpa"

type nameserver struct {
	FQDN string
	IP   string
}

type networkNameCIDRMap struct {
	name string
	cidr *net.IPNet
}

func makeDomainCanonical(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	} else {
		return fmt.Sprintf("%s.", domain)
	}
}

func ensureMasterZone(zoneName string, nameserverFQDNs []string,
	rrSets []powerdns.RRset) (masterZone *powerdns.Zone) {
	var err error
	masterZone, err = pdns.Zones.Get(zoneName)
	if err != nil {
		pdnsErr, ok := err.(*powerdns.Error)
		if !ok {
			logger.Error("Error not a PowerDNS error type!", zap.Error(err))
			return
		} else {
			if pdnsErr.StatusCode == http.StatusNotFound {
				zone := &powerdns.Zone{
					Name:        &zoneName,
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
				} else {
					logger.Info("Added master zone", zap.String("zoneName", zoneName))
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

func trueUpMasterZones(baseDomain string, networks []sls_common.Network,
	nameservers []nameserver) (masterZones []*powerdns.Zone) {
	// Compute all of the information we need concerning nameservers.
	var nameserverFQDNs []string
	var nameserverRRSets []powerdns.RRset

	// We have to create A records for all the name servers otherwise it won't let us create the zone.
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

		nameserverRRSets = append(nameserverRRSets, nameserverRRSet)
	}

	// First and foremost, check to make sure there is a master zone for the base domain. We won't add this to the
	// return set of master zones because none of the records should belong to the base domain, they should all be
	// fully qualified with their appropriate network.
	_ = ensureMasterZone(baseDomain, nameserverFQDNs, nameserverRRSets)
	//masterZones = append(masterZones, baseMasterZone)

	// Now make sure there is a master zone for each of the networks retrieved from SLS.
	for _, network := range networks {
		networkDomain := strings.ToLower(network.Name)
		fullDomain := fmt.Sprintf("%s.%s", networkDomain, baseDomain)

		networkMasterZone := ensureMasterZone(fullDomain, nameserverFQDNs, nil)
		if networkMasterZone != nil {
			masterZones = append(masterZones, networkMasterZone)
		}
	}

	return
}

func buildReverseName(cidrParts []string) string {
	var reverseCIDR []string

	for i := len(cidrParts) - 1; i >= 0; i-- {
		part := cidrParts[i]

		if part != "0" {
			reverseCIDR = append(reverseCIDR, cidrParts[i])
		}
	}

	return fmt.Sprintf("%s%s", strings.Join(reverseCIDR, "."), rdnsDomain)
}

func trueUpReverseZones(networks []sls_common.Network, nameservers []nameserver) (reverseZones []*powerdns.Zone,
	err error) {
	var nameserverFQDNs []string
	for _, nameserver := range nameservers {
		canonicalName := makeDomainCanonical(nameserver.FQDN)
		nameserverFQDNs = append(nameserverFQDNs, canonicalName)
	}
	for _, network := range networks {
		for _, ipRange := range network.IPRanges {
			// Compute the correct name.
			var cidr *net.IPNet
			_, cidr, err = net.ParseCIDR(ipRange)
			if err != nil {
				return
			}

			cidrParts := strings.Split(cidr.IP.String(), ".")
			reverseZoneName := buildReverseName(cidrParts)

			var reverseZone *powerdns.Zone
			reverseZone, err = pdns.Zones.Get(reverseZoneName)
			if err != nil {
				pdnsErr, ok := err.(*powerdns.Error)
				if !ok {
					logger.Error("Error not a PowerDNS error type!", zap.Error(err))
					return
				} else {
					if pdnsErr.StatusCode == http.StatusNotFound {
						reverseZone = &powerdns.Zone{
							Name:   &reverseZoneName,
							Kind:   powerdns.ZoneKindPtr(powerdns.MasterZoneKind),
							DNSsec: powerdns.Bool(true),
							Nameservers: nameserverFQDNs,
						}
						reverseZone, err = pdns.Zones.Add(reverseZone)
						if err != nil {
							logger.Error("Failed to add reverse zone!",
								zap.Error(err), zap.Any("reverseZone", *reverseZone))
							return
						} else {
							logger.Info("Added reverse zone", zap.Any("reverseZone", reverseZone))
						}
					}
				}
			}

			reverseZones = append(reverseZones, reverseZone)
		}
	}

	return
}

func buildStaticForwardRRSets(networks []sls_common.Network) (staticRRSets []powerdns.RRset, err error) {
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

				// Create the primary forward A record.
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

func buildStaticReverseRRSets(networks []sls_common.Network,
	reverseZone *powerdns.Zone) (staticReverseRRSets []powerdns.RRset, err error) {
	if !strings.Contains(*reverseZone.Name, rdnsDomain) {
		err = fmt.Errorf("zone does not appear to be a reverse zone: %s", *reverseZone.Name)
		return
	}

	// Compute the forward CIDR for this zone.
	trimmedCIDR := strings.TrimSuffix(*reverseZone.Name, rdnsDomain + ".")
	reverseCIDRParts := strings.Split(trimmedCIDR, ".")
	var forwardCIDR []string

	for i := len(reverseCIDRParts) - 1; i >= 0; i-- {
		forwardCIDR = append(forwardCIDR, reverseCIDRParts[i])
	}
	for i := len(forwardCIDR); i < 4; i++ {
		forwardCIDR = append(forwardCIDR, "0")
	}

	forwardCIDRString := strings.Join(forwardCIDR, ".")

	for _, network := range networks {
		for _, ipRange := range network.IPRanges {
			var ip net.IP
			ip, _, err = net.ParseCIDR(ipRange)
			if err != nil {
				return
			}

			if forwardCIDRString == ip.String() {
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
						cidrParts := strings.Split(reservation.IPAddress, ".")

						primaryRRsetReverse := powerdns.RRset{
							Name:       powerdns.String(makeDomainCanonical(buildReverseName(cidrParts))),
							Type:       powerdns.RRTypePtr(powerdns.RRTypePTR),
							TTL:        powerdns.Uint32(3600),
							ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
							Records: []powerdns.Record{
								{
									Content:  powerdns.String(primaryName),
									Disabled: powerdns.Bool(false),
								},
							},
						}
						staticReverseRRSets = append(staticReverseRRSets, primaryRRsetReverse)
					}
				}
			}
		}
	}

	return
}

func buildDynamicForwardRRsets(hardware []sls_common.GenericHardware, networks []sls_common.Network,
	ethernetInterfaces []sm.CompEthInterface) (dynamicRRSets []powerdns.RRset,
	err error) {

	// Start by precomputing network information.
	var networkNameCIDRMaps []networkNameCIDRMap
	for _, network := range networks {
		networkDomain := strings.ToLower(network.Name)

		for _, ipRange := range network.IPRanges {
			_, cidr, err := net.ParseCIDR(ipRange)
			if err != nil {
				logger.Error("Failed to parse network CIDR!", zap.Error(err), zap.Any("network", network))
			}

			networkNameCIDRMaps = append(networkNameCIDRMaps, networkNameCIDRMap{
				name: networkDomain,
				cidr: cidr,
			})
		}
	}

	// Also build an SLS hardware map.
	slsHardareMap := make(map[string]sls_common.GenericHardware)
	for _, device := range hardware {
		slsHardareMap[device.Xname] = device
	}

	for _, ethernetInterface := range ethernetInterfaces {
		// Have to ignore entries without IPs or ComponentIDs.
		if ethernetInterface.IPAddr == "" || ethernetInterface.CompID == "" {
			continue
		}

		var belongedNetwork networkNameCIDRMap
		ip, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", ethernetInterface.IPAddr))
		if err != nil {
			logger.Error("Failed to parse ethernet interface IP!",
				zap.Error(err), zap.Any("ethernetInterface", ethernetInterface))
			continue
		}

		// First figure out what network this IP belongs to.
		for _, network := range networkNameCIDRMaps {
			if network.cidr.Contains(ip) {
				belongedNetwork = network
				break
			}
		}

		if (belongedNetwork == networkNameCIDRMap{}) {
			logger.Error("Failed to find a network this ethernet interface belongs to!",
				zap.Any("ethernetInterface", ethernetInterface))
			continue
		}

		// Now we know the network path.
		networkDomain := strings.ToLower(belongedNetwork.name)

		// Start by making the core A record.
		primaryName := fmt.Sprintf("%s.%s.%s.", ethernetInterface.CompID, networkDomain, *baseDomain)
		primaryRRset := powerdns.RRset{
			Name:       powerdns.String(primaryName),
			Type:       powerdns.RRTypePtr(powerdns.RRTypeA),
			TTL:        powerdns.Uint32(3600),
			ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
			Records: []powerdns.Record{
				{
					Content:  powerdns.String(ethernetInterface.IPAddr),
					Disabled: powerdns.Bool(false),
				},
			},
		}
		dynamicRRSets = append(dynamicRRSets, primaryRRset)

		// Now we can create CNAME records for all of the aliases.
		// Start by getting the SLS hardware entry.
		slsEntry, found := slsHardareMap[ethernetInterface.CompID]
		if !found {
			logger.Error("Failed to find SLS entry for ethernet interface!",
				zap.Any("ethernetInterface", ethernetInterface))
			continue
		}

		var extraProperties sls_common.ComptypeNode
		err = mapstructure.Decode(slsEntry.ExtraPropertiesRaw, &extraProperties)
		if err != nil {
			logger.Error("Failed to decode node extra properties!", zap.Error(err))
			continue
		}

		for _, alias := range extraProperties.Aliases {
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
			dynamicRRSets = append(dynamicRRSets, aliasRRset)
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

func getZoneForRRSet(rrSet powerdns.RRset, zones []*powerdns.Zone) *string {
	for _, zone := range zones {
		if strings.HasSuffix(*rrSet.Name, *zone.Name) {
			return zone.Name
		}
	}

	return nil
}

// trueUpRRSets verifies all of the RRsets for the zone are as they should be.
// There are a total of 3 possibilities for each RRset:
//	1) The RRset doesn't exist at all.
//	2) The RRset exists but the records are not correct.
//  3) The RRset exists and shouldn't.
func trueUpRRSets(rrsets []powerdns.RRset, zones []*powerdns.Zone) (didSomething bool) {
	// Main data structure to keep track of the RRsets we actually need to patch with the zone it should be added to.
	actionableRRSetMap := make(map[string]*powerdns.RRsets)
	for _, zone := range zones {
		actionableRRSetMap[*zone.Name] = &powerdns.RRsets{Sets: []powerdns.RRset{}}
	}

	// To make this process a lot quicker first build up a map of names to RRsets for O(1) lookups later.
	zoneRRsetMap := make(map[string]powerdns.RRset)
	desiredRRSetMap := make(map[string]powerdns.RRset)

	for _, zone := range zones {
		for _, zoneRRset := range zone.RRsets {
			zoneRRsetMap[*zoneRRset.Name] = zoneRRset
		}
	}
	for _, desiredRRset := range rrsets {
		desiredRRSetMap[*desiredRRset.Name] = desiredRRset
	}

	for _, desiredRRset := range desiredRRSetMap {
		zoneRRset, found := zoneRRsetMap[*desiredRRset.Name]

		patchLogger := logger.With(zap.Any("desiredRRset", desiredRRset),
			zap.Any("zoneRRset", zoneRRset))

		// Need to identity which zone this record belongs to.
		zoneName := getZoneForRRSet(desiredRRset, zones)
		if zoneName == nil {
			patchLogger.Error("Desired RRSet did not match any master zones!", zap.Any("zones", zones))
			continue
		}


		zoneSets := &actionableRRSetMap[*zoneName].Sets

		if found {
			// Case 2 - is the RRSet correct?
			if !rrsetsEqual(desiredRRset, zoneRRset) {
				*zoneSets = append(*zoneSets, desiredRRset)
				patchLogger.Info("RRset exists but is not ideal configuration, adding to patch list.")
			} else {
				logger.Debug("RRset already at desired config", zap.Any("zoneRRset", zoneRRset))
			}
		} else {
			// Case 1 - not found, add it.
			*zoneSets = append(*zoneSets, desiredRRset)
			patchLogger.Info("RRset does not exist, adding to patch list.")
		}
	}

	// Case 3 - should this exist?
	for _, zoneRRset := range zoneRRsetMap {
		_, found := desiredRRSetMap[*zoneRRset.Name]

		if !found && zoneRRset.Type == powerdns.RRTypePtr(powerdns.RRTypeNS) {
			patchLogger := logger.With(zap.Any("zoneRRset", zoneRRset))

			// Need to identity which zone this record belongs to.
			zoneName := getZoneForRRSet(zoneRRset, zones)
			if zoneName == nil {
				patchLogger.Error("Desired RRSet did not match any master zones!", zap.Any("zones", zones))
				continue
			}

			zoneSets := actionableRRSetMap[*zoneName].Sets

			zoneRRset.ChangeType = powerdns.ChangeTypePtr(powerdns.ChangeTypeDelete)
			zoneSets = append(zoneSets, zoneRRset)
			patchLogger.Info("RRset needs to be removed, adding to patch list.")
		}
	}

	for zone, rrSets := range actionableRRSetMap {
		zoneLogger := logger.With(zap.String("zone", zone))

		if len(rrSets.Sets) > 0 {
			// Do all the patching (which is additions, changes, and deletes) in one API call...pretty cool.
			err := pdns.Records.Patch(zone, rrSets)
			if err != nil {
				zoneLogger.Error("Failed to patch RRsets!", zap.Error(err), zap.Any("zone", zone))
			} else {
				zoneLogger.Info("Patched RRSets")
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

		var finalRRSet []powerdns.RRset

		networks, err := getSLSNetworks()
		if err != nil {
			logger.Error("Failed to get networks from SLS!", zap.Error(err))
			continue
		}
		hardware, err := getSLSHardware()
		if err != nil {
			logger.Error("Failed to get hardware from SLS!", zap.Error(err))
			continue
		}
		ethernetInterfaces, err := getHSMEthernetInterfaces()
		if err != nil {
			logger.Error("Failed to get ethernet interfaces from HSM!", zap.Error(err))
			continue
		}

		// Build/get all necessary master zones.
		masterZones := trueUpMasterZones(*baseDomain, networks, nameServers)

		// True up reverse zones.
		reverseZones, err := trueUpReverseZones(networks, nameServers)
		if err != nil {
			logger.Error("Failed to true up reverse zones!", zap.Error(err))
		}

		staticRRSets, err := buildStaticForwardRRSets(networks)
		if err != nil {
			logger.Error("Failed to build static RRsets!", zap.Error(err))
		}
		for _, reverseZone := range reverseZones {
			staticRRSetsReverse, err := buildStaticReverseRRSets(networks, reverseZone)
			if err != nil {
				logger.Error("Failed to build reverse zone RRsets!",
					zap.Error(err), zap.Any("reverseZone", reverseZone))
			}

			if trueUpRRSets(staticRRSetsReverse, []*powerdns.Zone{reverseZone}) {
				result, err := pdns.Zones.Notify(*reverseZone.Name)
				if err != nil {
					logger.Error("Failed to notify slave server(s)!", zap.Error(err))
				} else {
					logger.Info("Notified slave server(s)", zap.Any("result", result))
				}
			}
		}

		dynamicRRSets, err := buildDynamicForwardRRsets(hardware, networks, ethernetInterfaces)
		if err != nil {
			logger.Error("Failed to build dynamic RRsets!", zap.Error(err))
		}

		// At this point we have computed every correct RRSet necessary. Now the only task is to add the ones that are
		// missing and remove the ones that shouldn't be there.

		finalRRSet = append(finalRRSet, staticRRSets...)
		finalRRSet = append(finalRRSet, dynamicRRSets...)

		// Force a sync to any slave servers if we did something.
		if trueUpRRSets(finalRRSet, masterZones) {
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
