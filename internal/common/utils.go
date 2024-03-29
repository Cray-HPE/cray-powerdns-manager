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
package common

import (
	"fmt"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/joeig/go-powerdns/v2"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func MakeDomainCanonical(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	} else {
		return fmt.Sprintf("%s.", domain)
	}
}

// GetReverseZoneName computes a reverse zone name from a slice of IPv4 parts.
func GetReverseZoneName(cidr *net.IPNet) string {
	var reverseCIDR []string
	var cidrParts []string

	prefix, _ := strconv.Atoi(strings.Split(cidr.String(), "/")[1])

	if prefix >= 24 {
		cidrParts = strings.Split(cidr.IP.String(), ".")[0:3]
	} else if prefix < 24 && prefix >= 16 {
		cidrParts = strings.Split(cidr.IP.String(), ".")[0:2]
	} else if prefix < 16 {
		cidrParts = strings.Split(cidr.IP.String(), ".")[0:1]
	}

	for i := len(cidrParts) - 1; i >= 0; i-- {
		reverseCIDR = append(reverseCIDR, cidrParts[i])
	}

	return fmt.Sprintf("%s%s", strings.Join(reverseCIDR, "."), rdnsDomain)
}

// GetForwardIP get the IP address from a reverse record name.
func GetForwardIP(reverseName string) string {
	var reverseParts []string
	var forwardIP []string

	reverseParts = strings.Split(strings.TrimSuffix(reverseName, ".in-addr.arpa."), ".")

	for i := len(reverseParts) - 1; i >= 0; i-- {
		forwardIP = append(forwardIP, reverseParts[i])
	}

	return strings.Join(forwardIP, ".")

}

// GetReverseName computes a reverse name from a slice of IPv4 parts.
func GetReverseName(cidrParts []string) string {
	var reverseCIDR []string

	for i := len(cidrParts) - 1; i >= 0; i-- {
		reverseCIDR = append(reverseCIDR, cidrParts[i])
	}

	return fmt.Sprintf("%s%s", strings.Join(reverseCIDR, "."), rdnsDomain)
}

// GetForwardCIDRStringForReverseZone generates a forward IP address from a reverse zone.
func GetForwardCIDRStringForReverseZone(reverseZone *powerdns.Zone) (forwardCIDRString string, err error) {
	if reverseZone == nil || reverseZone.Name == nil {
		err = fmt.Errorf("reverse zone or name is nil")
		return
	}
	if !strings.Contains(*reverseZone.Name, rdnsDomain) {
		err = fmt.Errorf("zone does not appear to be a reverse zone: %s", *reverseZone.Name)
		return
	}

	// Compute the forward CIDR for this zone.
	trimmedCIDR := strings.TrimSuffix(*reverseZone.Name, rdnsDomain+".")
	reverseCIDRParts := strings.Split(trimmedCIDR, ".")
	var forwardCIDR []string

	for i := len(reverseCIDRParts) - 1; i >= 0; i-- {
		forwardCIDR = append(forwardCIDR, reverseCIDRParts[i])
	}
	for i := len(forwardCIDR); i < 4; i++ {
		forwardCIDR = append(forwardCIDR, "0")
	}

	forwardCIDRString = strings.Join(forwardCIDR, ".")

	return
}

// GetNetworkForCIDRString computes the network and IP range for a CIDR string.
func GetNetworkForCIDRString(networks []sls_common.Network, cidr string) (*sls_common.Network, *string) {
	for _, network := range networks {
		for _, ipRange := range network.IPRanges {
			if strings.HasPrefix(ipRange, cidr) {
				return &network, &ipRange
			}
		}
	}

	return nil, nil
}

func RRsetsEqual(a powerdns.RRset, b powerdns.RRset) bool {
	if *a.Name != *b.Name ||
		!reflect.DeepEqual(a.Records, b.Records) ||
		*a.TTL != *b.TTL ||
		*a.Type != *b.Type {
		return false
	}

	return true
}

func RRsetsContains(a []powerdns.RRset, b powerdns.RRset) bool {

	for _, rrset := range a {
		if RRsetsEqual(rrset, b) {
			return true
		}
	}
	return false
}

// Sorting functions for arrays of zones.

func (zones PowerDNSZones) Len() int {
	return len(zones)
}
func (zones PowerDNSZones) Swap(i, j int) {
	zones[i], zones[j] = zones[j], zones[i]
}
func (zones PowerDNSZones) Less(i, j int) bool {
	iName := *zones[i].Name
	jName := *zones[j].Name

	// When trying to find the correct zone the goal is always to find the zone with the maximal matching suffix.
	// To do this, we apply a naive approach algorithmically and sort the zones by length.
	// Given two zones i and j with common suffix if the length of i is less than j then by definition i has a
	// potential greater suffix length than j. In a sequential search of a list of strings sorted by length descending
	// we can be guaranteed that the maximally matching zone will appear first.
	return len(iName) > len(jName)
}

func GetZoneForRRSet(rrSet powerdns.RRset, zones PowerDNSZones) *string {
	// Have to reverse sort the zones to make sure we match maximal prefix first.
	sort.Sort(zones)

	for _, zone := range zones {
		if strings.HasSuffix(*rrSet.Name, *zone.Name) {
			return zone.Name
		}
	}

	return nil
}

func GetNameserverRRset(nameserver Nameserver) powerdns.RRset {
	return powerdns.RRset{
		Name:       powerdns.String(MakeDomainCanonical(nameserver.FQDN)),
		Type:       powerdns.RRTypePtr(powerdns.RRTypeA),
		TTL:        powerdns.Uint32(3600),
		ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
		Records: []powerdns.Record{
			{
				Content:  powerdns.String(nameserver.IP),
				Disabled: powerdns.Bool(false),
			},
		},
	}
}

func GetDNAMERRSet(masterZoneName string, baseDomain string, masterZoneNames []string) (rrSet powerdns.RRset, err error) {
	for _, zone := range masterZoneNames {
		if strings.HasPrefix(zone, MakeDomainCanonical(masterZoneName)) && strings.HasSuffix(zone, baseDomain) {

			rrSet = powerdns.RRset{
				Name:       powerdns.String(MakeDomainCanonical(masterZoneName)),
				Type:       powerdns.RRTypePtr(powerdns.RRTypeDNAME),
				TTL:        powerdns.Uint32(3600),
				ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
				Records: []powerdns.Record{
					{
						Content:  powerdns.String(MakeDomainCanonical(zone)),
						Disabled: powerdns.Bool(false),
					},
				},
			}
			return
		}
	}
	err = fmt.Errorf("Did not find fully qualified domain for short name: %s", masterZoneName)
	return
}

func GetStartOfAuthorityRRSet(zoneName string,
	mname string,
	rname string,
	refresh string,
	retry string,
	expire string,
	ttl string) powerdns.RRset {

	// Generate a SOA record that has the correct nameserver name
	soaString := fmt.Sprintf("%s %s %s %s %s %s %s",
		mname,
		rname,
		"0", // Serial
		refresh,
		retry,
		expire,
		ttl,
	)

	return powerdns.RRset{
		Name:       powerdns.String(MakeDomainCanonical(zoneName)),
		Type:       powerdns.RRTypePtr(powerdns.RRTypeSOA),
		TTL:        powerdns.Uint32(3600),
		ChangeType: powerdns.ChangeTypePtr(powerdns.ChangeTypeReplace),
		Records: []powerdns.Record{
			{
				Content:  powerdns.String(soaString),
				Disabled: powerdns.Bool(false),
			},
		},
	}
}

func SliceContains(needle string, haystack []string) bool {
	for _, match := range haystack {
		if needle == match {
			return true
		}
	}

	return false
}
