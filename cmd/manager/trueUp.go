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
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Cray-HPE/cray-powerdns-manager/internal/common"
	base "github.com/Cray-HPE/hms-base"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/Cray-HPE/hms-smd/pkg/sm"
	"github.com/joeig/go-powerdns/v2"
	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
)

func ensureMasterZone(zoneName string, nameserverFQDNs []string, rrSets []powerdns.RRset) (masterZone *powerdns.Zone) {
	var err error
	masterZone, err = pdns.Zones.Get(zoneName)
	if err != nil {
		masterZone = nil

		pdnsErr, ok := err.(*powerdns.Error)
		if !ok {
			logger.Error("Error not a PowerDNS error type!", zap.Error(err))
			return
		} else {
			if pdnsErr.StatusCode == http.StatusNotFound {
				// Figure out if this zone has a custom DNSSEC key.
				var customDNSSECKey *common.DNSKey
				var tsigKeyIDs []string
				for _, key := range DNSKeys {
					if strings.TrimSuffix(zoneName, ".") == key.Name {
						// Required because the loop variable itself is a reference.
						tmpKey := key
						customDNSSECKey = &tmpKey
					}
					if key.Type == common.TSIGKeyType {
						tsigKeyIDs = append(tsigKeyIDs, key.Name)
					}
				}

				zone := &powerdns.Zone{
					Name:             &zoneName,
					Kind:             powerdns.ZoneKindPtr(powerdns.MasterZoneKind),
					DNSsec:           powerdns.Bool(false),
					Nameservers:      nameserverFQDNs,
					RRsets:           rrSets,
					MasterTSIGKeyIDs: tsigKeyIDs,
				}

				masterZone, err = pdns.Zones.Add(zone)
				if err != nil {
					logger.Error("Failed to add master zone!",
						zap.Error(err), zap.Any("masterZone", *zone))
					return
				} else {
					logger.Info("Added master zone", zap.String("zoneName", zoneName))
					// Rectify the zone to ensure all DNSSEC data is correct
					_, err = pdns.Zones.Rectify(zoneName)
					if err != nil {
						logger.Error("Failed to rectify zone", zap.String("zoneName", zoneName), zap.Error(err))
					}

					// Now that the zone is added we check to see if we found a custom DNSSEC key and if so upload that.
					if customDNSSECKey != nil {
						err = AddCryptokeyToZone(*customDNSSECKey)

						if err != nil {
							logger.Error("Failed to add custom DNSSEC key to zone!", zap.Error(err))
						}
					}
				}

				return
			} else {
				logger.Error("Got unexpected status code from attempt to get zone!", zap.Error(err))
				return
			}
		}

		// TODO: Add logic to update the master zone if necessary.
	}

	logger.Debug("Master zone already exists.", zap.String("masterZone.name", *masterZone.Name))

	return
}

func trueUpMasterZones(baseDomain string, networks []sls_common.Network,
	masterNameserver common.Nameserver, slaveNameservers []common.Nameserver) (masterZones []*powerdns.Zone) {
	// Create a list of all the master zones.
	masterZoneNames := []string{baseDomain}
	for _, network := range networks {
		networkDomain := strings.ToLower(network.Name)
		fullDomain := fmt.Sprintf("%s.%s", networkDomain, baseDomain)

		masterZoneNames = append(masterZoneNames, fullDomain)
		if *createDNAME == true {
			masterZoneNames = append(masterZoneNames, networkDomain)
		}
	}

	// Every zone should have at least the master nameserver.
	masterNameserverRRSet := common.GetNameserverRRset(masterNameserver)
	baseNameserverFQDNs := []string{*masterNameserverRRSet.Name}

	for _, masterZoneName := range masterZoneNames {
		nameserverFQDNs := baseNameserverFQDNs
		var nameserverRRSets []powerdns.RRset

		// If this is is the base domain we need treat it a little differently in that we need to include RRsets for
		// the A record of the master server otherwise it won't let us create the zone.
		if masterZoneName == baseDomain {
			nameserverRRSets = append(nameserverRRSets, masterNameserverRRSet)

			// Add the subdomain delegation NS records to the TLD.
			for _, zone := range masterZoneNames {
				if zone == baseDomain {
					continue
				}

				// Don't want to include any of the DNAME zones
				if strings.HasSuffix(zone, baseDomain) {
					logger.Debug("Add NS records", zap.Any("masterZoneName", zone))
					ns := powerdns.RRset{
						Name:       powerdns.String(common.MakeDomainCanonical(zone)),
						Type:       powerdns.RRTypePtr(powerdns.RRTypeNS),
						TTL:        powerdns.Uint32(3600),
						ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
						Records: []powerdns.Record{
							{
								Content:  powerdns.String(common.MakeDomainCanonical(masterNameserver.FQDN)),
								Disabled: powerdns.Bool(false),
							},
						},
					}
					nameserverRRSets = append(nameserverRRSets, ns)
				}
			}
		}

		// This is a short zone that requires a DNAME pointer to the fully qualified zone
		if !strings.HasSuffix(masterZoneName, baseDomain) && *createDNAME == true {
			logger.Debug("Found short zone name, creating DNAME record", zap.String("masterZoneName", masterZoneName))
			dnameRRSet, err := common.GetDNAMERRSet(masterZoneName, baseDomain, masterZoneNames)
			if err == nil {
				logger.Debug("Adding DNAME RRSet to zone", zap.Any("RRSet", dnameRRSet))
				nameserverRRSets = append(nameserverRRSets, dnameRRSet)
			} else {
				logger.Error("Cannot find fully qualified domain for short name. Unable to create DNAME record",
					zap.String("masterZoneName", masterZoneName))
			}
		}

		// Generate a SOA record that has the correct nameserver name
		soa := common.GetStartOfAuthorityRRSet(masterZoneName,
			*masterNameserverRRSet.Name,
			fmt.Sprintf("hostmaster.%s.", masterZoneName),
			*soaRefresh,
			*soaRetry,
			*soaExpiry,
			*soaMinimum,
		)

		nameserverRRSets = append(nameserverRRSets, soa)

		// Now figure out if this zone is enabled for zone transfers and if so add the slave server(s) to the
		// name server list.
		if len(notifyZonesArray) == 0 || common.SliceContains(masterZoneName, notifyZonesArray) {
			for _, nameserver := range slaveNameservers {
				nameserverFQDNs = append(nameserverFQDNs, nameserver.FQDN)
			}
		}

		masterZone := ensureMasterZone(masterZoneName, nameserverFQDNs, nameserverRRSets)
		if masterZone != nil && masterZone.ID != nil {
			masterZones = append(masterZones, masterZone)
		}
	}

	// TODO: Remove any zones that don't correspond to networks from SLS.

	return
}

func trueUpReverseZones(networks []sls_common.Network,
	masterNameserver common.Nameserver, slaveNameservers []common.Nameserver) (reverseZones []*powerdns.Zone,
	err error) {
networks:
	for _, network := range networks {
		for _, ipRange := range network.IPRanges {
			var nameserverFQDNs []string
			var nameserverRRSets []powerdns.RRset

			// Compute the correct name.
			var cidr *net.IPNet
			_, cidr, err = net.ParseCIDR(ipRange)
			if err != nil {
				return
			}

			reverseZoneName := common.GetReverseZoneName(cidr)
			logger.Debug("Calculated reverse zone name:", zap.Any("sls_network", network.Name),
				zap.Any("cidr", cidr), zap.Any("reverseZoneName", reverseZoneName))

			/*
				As reverse zones split on a /24 boundary it's possible for two SLS subnets to map to the same reverse
				zone. For example a CAN of 10.101.5.128/26 and a CMN of 10.101.5.0/25 would map to the same
				5.101.10.in-addr.arpa zone. This avoids adding the same zone to the reverseZones array twice
			*/
			for _, zone := range reverseZones {
				if strings.Contains(*zone.Name, reverseZoneName) {
					logger.Debug("Master zone already exists.", zap.String("reverseZoneName", reverseZoneName))
					continue networks
				}
			}

			// This master name server is always listed as one of the nameservers.
			masterNameserverRRSet := common.GetNameserverRRset(masterNameserver)
			nameserverFQDNs = append(nameserverFQDNs, *masterNameserverRRSet.Name)

			// Build valid SOA record
			soa := common.GetStartOfAuthorityRRSet(reverseZoneName,
				*masterNameserverRRSet.Name,
				fmt.Sprintf("hostmaster.%s", common.MakeDomainCanonical(*baseDomain)),
				*soaRefresh,
				*soaRetry,
				*soaExpiry,
				*soaMinimum,
			)
			nameserverRRSets = append(nameserverRRSets, soa)

			// Now figure out if this zone is enabled for zone transfers and if so add the slave server(s) to the
			// name server list.
			if len(notifyZonesArray) == 0 || common.SliceContains(reverseZoneName, notifyZonesArray) {
				for _, nameserver := range slaveNameservers {
					nameserverRRSet := common.GetNameserverRRset(nameserver)

					nameserverFQDNs = append(nameserverFQDNs, *nameserverRRSet.Name)
				}
			}

			var reverseZone *powerdns.Zone
			reverseZone, err = pdns.Zones.Get(reverseZoneName)
			if err == nil {
				// TODO: Add logic to make sure all the details are correct and remove reverse zones if necessary.
				reverseZones = append(reverseZones, reverseZone)
			} else {
				pdnsErr, ok := err.(*powerdns.Error)
				if !ok {
					logger.Error("Error not a PowerDNS error type!", zap.Error(err))
					return
				} else {
					if pdnsErr.StatusCode == http.StatusNotFound {
						// Figure out if this zone has any custom keys.
						var customDNSSECKey *common.DNSKey
						var tsigKeyIDs []string
						for _, key := range DNSKeys {
							if strings.TrimSuffix(reverseZoneName, ".") == key.Name {
								// Required because the loop variable itself is a reference.
								tmpKey := key
								customDNSSECKey = &tmpKey
							}
							if key.Type == common.TSIGKeyType {
								tsigKeyIDs = append(tsigKeyIDs, key.Name)
							}
						}

						reverseZone = &powerdns.Zone{
							Name:             &reverseZoneName,
							Kind:             powerdns.ZoneKindPtr(powerdns.MasterZoneKind),
							DNSsec:           powerdns.Bool(false),
							Nameservers:      nameserverFQDNs,
							MasterTSIGKeyIDs: tsigKeyIDs,
							RRsets:           nameserverRRSets,
						}
						reverseZone, err = pdns.Zones.Add(reverseZone)
						if err != nil {
							logger.Error("Failed to add reverse zone!",
								zap.Error(err), zap.Any("reverseZone", *reverseZone))
							return
						} else {
							logger.Info("Added reverse zone", zap.Any("reverseZone", reverseZone))
							reverseZones = append(reverseZones, reverseZone)

							// Now that the zone is added we check to see if we found a custom DNSSEC key and if so
							// upload that.
							if customDNSSECKey != nil {
								err = AddCryptokeyToZone(*customDNSSECKey)

								if err != nil {
									logger.Error("Failed to add custom DNSSEC key to reverse zone!",
										zap.Error(err))
								}
							}
						}
					} else {
						logger.Error("Got unknown PowerDNS error!", zap.Any("pdnsErr", pdnsErr))
					}
				}
			}
		}
	}

	return
}

func buildStaticForwardRRSets(networks []sls_common.Network, hardware []sls_common.GenericHardware, state base.ComponentArray) (
	staticRRSets []powerdns.RRset, err error) {
	// Build up a map of the hardware to save lookup time later.
	hardwareMap := make(map[string]sls_common.GenericHardware)
	for _, device := range hardware {
		hardwareMap[device.Xname] = device
	}

	// Build up a map of the state data to avoid having to iterate through it for the HSN and CHN records.
	stateMap := make(map[string]base.Component)
	for _, node := range state.Components {
		stateMap[node.ID] = *node
	}

	for _, network := range networks {
		networkDomain := strings.ToLower(network.Name)

		var networkProperties NetworkExtraProperties
		err = mapstructure.Decode(network.ExtraPropertiesRaw, &networkProperties)
		if err != nil {
			return
		}

		for _, subnet := range networkProperties.Subnets {
			for _, reservation := range subnet.IPReservations {
				// Can't believe this is a thing, but, for some reason the xname for some entries is in the comment
				// field. If that's the case, then we create the A record from that and then a CNAME for the name
				// and then CNAMEs for each of the aliases.
				// Start by seeing if this comment corresponds to a hardware object (i.e., is an xname).
				node, found := hardwareMap[reservation.Comment]

				// Now we can build the primary name for the A record.
				var primaryName string
				if found {
					primaryName = fmt.Sprintf("%s.%s.%s.", node.Xname, networkDomain, *baseDomain)

					// In this case we also have to create an additional RRset for the name which is the primary alias
					// for the node...yea, still kooky.
					nameRRset := powerdns.RRset{
						Name: powerdns.String(fmt.Sprintf("%s.%s.%s.",
							reservation.Name, networkDomain, *baseDomain)),
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
					staticRRSets = append(staticRRSets, nameRRset)
				} else {
					primaryName = fmt.Sprintf("%s.%s.%s.", reservation.Name, networkDomain, *baseDomain)
				}

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
					/* Avoid bad names such as .local etc.
					   The SLS aliases can contain the node xname which results in this
					   rather unfortunate DNS record if not removed

					   x3000c0s3b0n0.nmn.drax.dev.cray.com	3600	IN	CNAME	x3000c0s3b0n0.nmn.drax.dev.cray.com.  */
					if strings.Contains(alias, ".") || alias == node.Xname {
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
				/*
					Now create HSN/CHN specific nid aliases
					nid000001.chn.<system domain>
					nid000001.hsn.<system domain>
					nid000001-hsn0.hsn.<system domain>
				*/
				switch {
				case networkDomain == "hsn":
					logger.Debug("Processing HSN network, create alias records")
					hostname, nic, e := getHSNNidNic(reservation.Name, hardwareMap, stateMap)
					if e != nil {
						logger.Error("Unable to determine HSN NID alias", zap.Any("error", e))
						continue
					}
					logger.Debug("Got alias and NIC data", zap.String("hostname", hostname), zap.Int("nic", nic))
					hsnname := fmt.Sprintf("%s-hsn%d", hostname, nic)
					logger.Debug("Create HSN NIC host alias", zap.Any("xname", reservation.Name), zap.Any("alias", hsnname))

					aliasRRset := powerdns.RRset{
						Name:       powerdns.String(fmt.Sprintf("%s.%s.%s.", hsnname, networkDomain, *baseDomain)),
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

					// If the HSN nic index is 0, create the extra nid record for the host
					if nic == 0 {
						logger.Debug("Create HSN host alias", zap.Any("hostname", hostname), zap.Int("nic", nic))

						aliasRRset := powerdns.RRset{
							Name:       powerdns.String(fmt.Sprintf("%s.%s.%s.", hostname, networkDomain, *baseDomain)),
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
				case networkDomain == "chn":
					hostname, _, e := getHSNNidNic(reservation.Name, hardwareMap, stateMap)
					if e != nil {
						// This is logged at debug level rather than error because the CHN network
						// has other aliases that aren't xnames (chn-switch-1, ncn-m001 etc.) that
						// will cause the lookup to always fail resulting in a noisy log.
						logger.Debug("Unable to determine CHN hostname", zap.Any("error", e))
						continue
					}
					logger.Debug("Got CHN hostname", zap.String("hostname", hostname))

					aliasRRset := powerdns.RRset{
						Name:       powerdns.String(fmt.Sprintf("%s.%s.%s.", hostname, networkDomain, *baseDomain)),
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

func buildDynamicReverseRRSets(networks []sls_common.Network, ethernetInterfaces []sm.CompEthInterfaceV2) (dynamicRRSets []powerdns.RRset, err error) {

	// Loop round SMD ethernetInterfaces and then build a list of rrSets
	for _, ethernetInterface := range ethernetInterfaces {
		if len(ethernetInterface.IPAddrs) == 0 || ethernetInterface.CompID == "" {
			// Can't process entries that don't have an IP or component ID.
			continue
		}

		for _, ethernetIP := range ethernetInterface.IPAddrs {

			var ip net.IP
			ip, _, err = net.ParseCIDR(fmt.Sprintf("%s/32", ethernetIP.IPAddr))
			if err != nil {
				logger.Debug("Failed to parse ethernet interface IP address!",
					zap.Error(err), zap.Any("ethernetInterface", ethernetInterface))
				continue
			}

			// Figure out what SLS network we're in
			var exists bool = false
			for _, network := range networks {
				var networkProperties NetworkExtraProperties
				err = mapstructure.Decode(network.ExtraPropertiesRaw, &networkProperties)
				if err != nil {
					return
				}
				var forwardCIDR *net.IPNet
				_, forwardCIDR, err = net.ParseCIDR(networkProperties.CIDR)
				if err != nil {
					return
				}

				networkDomain := strings.ToLower(network.Name)

				if forwardCIDR.Contains(ip) {
					exists = true
					logger.Debug("buildDynamicReverseRRSets: Network membership found",
						zap.Any("network", network.Name),
						zap.Any("forwardCIDR", forwardCIDR), zap.Any("IP", ip))

					reverseZoneName := common.GetReverseZoneName(forwardCIDR)
					cidrParts := strings.Split(ip.String(), ".")

					logger.Debug("buildDynamicReverseRRSets: Calculated reverse zone membership",
						zap.Any("reverseZoneName", reverseZoneName),
						zap.Any("reverseName", common.GetReverseName(cidrParts)))

					primaryName := fmt.Sprintf("%s.%s.%s.", ethernetInterface.CompID, networkDomain, *baseDomain)

					rrsetReverse := powerdns.RRset{
						Name:       powerdns.String(common.MakeDomainCanonical(common.GetReverseName(cidrParts))),
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

					dynamicRRSets = append(dynamicRRSets, rrsetReverse)

				}
			}
			if exists == false {
				logger.Debug("buildDynamicReverseRRSets: ethernetInterfaces record does not belong to any SLS network",
					zap.Any("ethernetInterfaces", ethernetInterface))
			}
		}
	}
	return
}

func buildStaticReverseRRSets(networks []sls_common.Network,
	reverseZone *powerdns.Zone) (staticReverseRRSets []powerdns.RRset, err error) {
	var forwardCIDRString string
	forwardCIDRString, err = common.GetForwardCIDRStringForReverseZone(reverseZone)
	if err != nil {
		return
	}

	for _, network := range networks {
		for _, ipRange := range network.IPRanges {
			var ip net.IP
			ip, _, err = net.ParseCIDR(ipRange)
			if err != nil {
				return
			}

			/*	Need to discount the last octet as on a system with small subnets multiple SLS networks could map to
				the	same reverse zone. For example a CAN of 10.101.5.128/26 and a CMN of 10.101.5.0/25 would both map
				to the same /24 reverse zone of 5.101.10.in-addr.arpa */
			ip = ip.To4()
			ip[3] = 0

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

						rrsetReverse := powerdns.RRset{
							Name:       powerdns.String(common.MakeDomainCanonical(common.GetReverseName(cidrParts))),
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
						staticReverseRRSets = append(staticReverseRRSets, rrsetReverse)
					}
				}
			}
		}
	}

	return
}

func buildDynamicForwardRRsets(hardware []sls_common.GenericHardware, networks []sls_common.Network,
	ethernetInterfaces []sm.CompEthInterfaceV2) (dynamicRRSets []powerdns.RRset,
	err error) {

	// Start by precomputing network information.
	var networkNameCIDRMaps []common.NetworkNameCIDRMap
	for _, network := range networks {
		networkDomain := strings.ToLower(network.Name)

		for _, ipRange := range network.IPRanges {
			_, cidr, err := net.ParseCIDR(ipRange)
			if err != nil {
				logger.Error("Failed to parse network CIDR!", zap.Error(err), zap.Any("network", network))
			}

			networkNameCIDRMaps = append(networkNameCIDRMaps, common.NetworkNameCIDRMap{
				Name: networkDomain,
				CIDR: cidr,
			})
		}
	}

	// Also build an SLS hardware map.
	slsHardwareMap := make(map[string]sls_common.GenericHardware)
	for _, device := range hardware {
		slsHardwareMap[device.Xname] = device
	}

	for _, ethernetInterface := range ethernetInterfaces {
		// Have to ignore entries without IPs or ComponentIDs.
		if len(ethernetInterface.IPAddrs) == 0 || ethernetInterface.CompID == "" {
			continue
		}

		for _, ethernetIP := range ethernetInterface.IPAddrs {

			var belongedNetwork common.NetworkNameCIDRMap
			ip, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", ethernetIP.IPAddr))
			if err != nil {
				logger.Debug("Failed to parse ethernet interface IP!",
					zap.Error(err), zap.Any("ethernetInterface", ethernetInterface))
				continue
			}

			// First figure out what network this IP belongs to.
			for _, network := range networkNameCIDRMaps {
				if network.CIDR.Contains(ip) {
					belongedNetwork = network
					break
				}
			}

			if (belongedNetwork == common.NetworkNameCIDRMap{}) {
				logger.Error("Failed to find a network this ethernet interface belongs to!",
					zap.Any("ethernetInterface", ethernetInterface))
				continue
			}

			// Now we know the network path.
			networkDomain := strings.ToLower(belongedNetwork.Name)

			// Start by making the core A record.
			primaryName := fmt.Sprintf("%s.%s.%s.", ethernetInterface.CompID, networkDomain, *baseDomain)
			primaryRRset := powerdns.RRset{
				Name:       powerdns.String(primaryName),
				Type:       powerdns.RRTypePtr(powerdns.RRTypeA),
				TTL:        powerdns.Uint32(3600),
				ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
				Records: []powerdns.Record{
					{
						Content:  powerdns.String(ethernetIP.IPAddr),
						Disabled: powerdns.Bool(false),
					},
				},
			}
			dynamicRRSets = append(dynamicRRSets, primaryRRset)

			// Now we can create CNAME records for all of the aliases.
			// Start by getting the SLS hardware entry.
			slsEntry, found := slsHardwareMap[ethernetInterface.CompID]
			if !found {
				logger.Debug("Failed to find SLS entry for ethernet interface!",
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
	}

	return
}

// trueUpRRSets verifies all of the RRsets for the zone are as they should be.
// There are a total of 3 possibilities for each RRset:
//  1. The RRset doesn't exist at all.
//  2. The RRset exists but the records are not correct.
//  3. The RRset exists and shouldn't.
func trueUpRRSets(rrsets []powerdns.RRset, zones []*powerdns.Zone) (didSomething bool) {
	// Main data structure to keep track of the RRsets we actually need to patch with the zone it should be added to.
	actionableRRSetMap := make(map[string]*powerdns.RRsets)
	for _, zone := range zones {
		actionableRRSetMap[*zone.Name] = &powerdns.RRsets{Sets: []powerdns.RRset{}}
	}

	// To make this process a lot quicker first build up a map of names to RRsets for O(1) lookups later.
	zoneRRsetMap := make(map[string]powerdns.RRset)
	desiredRRSetMap := make(map[string]powerdns.RRset)

	var zoneNames []string
	for _, zone := range zones {
		zoneNames = append(zoneNames, *zone.Name)

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
		zoneName := common.GetZoneForRRSet(desiredRRset, zones)
		if zoneName == nil {
			patchLogger.Error("Desired RRSet did not match any master zones!",
				zap.Any("zoneNames", zoneNames))
			continue
		}

		zoneSets := &actionableRRSetMap[*zoneName].Sets

		if found {
			// Case 2 - is the RRSet correct?
			if !common.RRsetsEqual(desiredRRset, zoneRRset) {
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
			zoneName := common.GetZoneForRRSet(zoneRRset, zones)
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

func trueUpDNS() {
	logger.Info("Running true up loop at interval.", zap.Int("trueUpLoopInterval", *trueUpSleepInterval))

	defer WaitGroup.Done()

	var masterNameserver common.Nameserver
	var slaveNameservers []common.Nameserver

	if *masterServer != "" {
		masterNameserverSplit := strings.Split(*masterServer, "/")
		if len(masterNameserverSplit) != 2 {
			logger.Fatal("Master nameserver does not have name/IP format!",
				zap.String("masterServer", *masterServer))
		}
		masterNameserver = common.Nameserver{
			FQDN: fmt.Sprintf("%s.%s", masterNameserverSplit[0], *baseDomain),
			IP:   masterNameserverSplit[1],
		}
	}

	if *slaveServers != "" {
		for _, slaveServer := range strings.Split(*slaveServers, ",") {
			nameserverSplit := strings.Split(slaveServer, "/")
			if len(nameserverSplit) != 2 {
				logger.Fatal("Slave nameserver does not have FQDN/IP format!",
					zap.String("slaveServer", slaveServer))
			}
			slaveNameserver := common.Nameserver{
				FQDN: common.MakeDomainCanonical(nameserverSplit[0]),
				IP:   nameserverSplit[1],
			}

			slaveNameservers = append(slaveNameservers, slaveNameserver)
		}
	}

	for Running {
		// This block is at the very top of this loop so that we can `continue` our way to the next iteration if there
		// is an error and not just blow past the sleep block.
		select {
		case <-trueUpShutdown:
			return
		case <-trueUpRunNow: // For those impatient types.
		case <-time.After(time.Duration(*trueUpSleepInterval) * time.Second):
			logger.Debug("Running true up loop.")
		}

		trueUpMtx.Lock()
		trueUpInProgress = true
		trueUpMtx.Unlock()

		var allMasterZones common.PowerDNSZones
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

		// Retrieve smd/v2/State/Components records. Necessary because the UAN NID is dynamically assigned by SMD.
		stateComponents, err := getHSMNodeState()
		if err != nil {
			logger.Error("Failed to get component state from HSM!", zap.Error(err))
			continue
		}

		// Build/get all necessary master zones.
		masterZones := trueUpMasterZones(*baseDomain, networks, masterNameserver, slaveNameservers)

		// True up reverse zones.
		reverseZones, err := trueUpReverseZones(networks, masterNameserver, slaveNameservers)
		if err != nil {
			logger.Error("Failed to true up reverse zones!", zap.Error(err))
		}

		// Build a list of all master zones both forward and reverse.
		allMasterZones = append(allMasterZones, masterZones...)
		allMasterZones = append(allMasterZones, reverseZones...)

		// Build the RRSets, static SLS records first then the HSM dynamic records.
		// The PowerDNS API will not permit the submission of duplicates so drop entries
		// that already exist in finalRRSet before passing to trueUpRRSets() to make the
		// API call.
		staticRRSets, err := buildStaticForwardRRSets(networks, hardware, stateComponents)
		if err != nil {
			logger.Error("Failed to build static RRsets!", zap.Error(err))
		}
		finalRRSet = append(finalRRSet, staticRRSets...)

		for _, reverseZone := range reverseZones {
			staticRRSetsReverse, err := buildStaticReverseRRSets(networks, reverseZone)
			if err != nil {
				logger.Error("Failed to build reverse zone RRsets!",
					zap.Error(err), zap.Any("reverseZone", reverseZone))
			}

			// Add all these records to the final RR set.
			finalRRSet = append(finalRRSet, staticRRSetsReverse...)
		}

		dynamicRRSets, err := buildDynamicForwardRRsets(hardware, networks, ethernetInterfaces)
		if err != nil {
			logger.Error("Failed to build dynamic RRsets!", zap.Error(err))
		}

		for _, RRSet := range dynamicRRSets {
			if !common.RRsetsContains(finalRRSet, RRSet) {
				logger.Debug("Adding RRset", zap.Any("rrSet", RRSet))
				finalRRSet = append(finalRRSet, RRSet)
			} else {
				logger.Debug("Refusing to add duplicate RRset", zap.Any("rrSet", RRSet))
			}

		}

		dynamicRRSetsReverse, err := buildDynamicReverseRRSets(networks, ethernetInterfaces)
		if err != nil {
			logger.Error("Failed to build reverse zone RRsets!",
				zap.Error(err))
		}

		for _, RRSet := range dynamicRRSetsReverse {
			if !common.RRsetsContains(finalRRSet, RRSet) {
				logger.Debug("Adding RRset", zap.Any("rrSet", RRSet))
				finalRRSet = append(finalRRSet, RRSet)
			} else {
				logger.Debug("Refusing to add duplicate RRset", zap.Any("rrSet", RRSet))
			}

		}

		// At this point we have computed every correct RRSet necessary. Now the only task is to add the ones that are
		// missing and remove the ones that shouldn't be there.
		// TODO: Add the remove entries.

		// Force a sync to any slave servers if we did something.
		if trueUpRRSets(finalRRSet, allMasterZones) {
			for _, masterZone := range allMasterZones {
				result, err := pdns.Zones.Notify(*masterZone.Name)

				notifyLogger := logger.With(zap.String("masterZone.name", *masterZone.Name))
				if err != nil {
					notifyLogger.Error("Failed to notify slave server(s) for zone!", zap.Error(err))
				} else {
					notifyLogger.Info("Notified slave server(s) for zone", zap.Any("result", result))
				}
			}
		}

		trueUpMtx.Lock()
		trueUpInProgress = false
		trueUpMtx.Unlock()
	}

	logger.Info("True up loop shutdown.")
}
