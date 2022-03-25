package common

import (
	"fmt"
	"github.com/joeig/go-powerdns/v2"
	"net"
	"reflect"
	"sort"
	sls_common "stash.us.cray.com/HMS/hms-sls/pkg/sls-common"
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

	return fmt.Sprintf("%s%s", strings.Join(reverseCIDR, "."), ".in-addr.arpa")
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

func SliceContains(needle string, haystack []string) bool {
	for _, match := range haystack {
		if needle == match {
			return true
		}
	}

	return false
}
