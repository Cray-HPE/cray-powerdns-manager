package common

import (
	"fmt"
	"github.com/joeig/go-powerdns/v2"
	"reflect"
	sls_common "stash.us.cray.com/HMS/hms-sls/pkg/sls-common"
	"strings"
)

func MakeDomainCanonical(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	} else {
		return fmt.Sprintf("%s.", domain)
	}
}

// GetReverseName computes a reverse name from a slice of IPv4 parts.
func GetReverseName(cidrParts []string) string {
	var reverseCIDR []string

	for i := len(cidrParts) - 1; i >= 0; i-- {
		part := cidrParts[i]

		if part != "0" {
			reverseCIDR = append(reverseCIDR, cidrParts[i])
		}
	}

	return fmt.Sprintf("%s%s", strings.Join(reverseCIDR, "."), rdnsDomain)
}

// GetForwardCIDRStringForReverseZone generates a forward IP address from a reverse zone.
func GetForwardCIDRStringForReverseZone(reverseZone *powerdns.Zone) (forwardCIDRString string, err error) {
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

func GetZoneForRRSet(rrSet powerdns.RRset, zones []*powerdns.Zone) *string {
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
				Content:  powerdns.String(MakeDomainCanonical(nameserver.FQDN)),
				Disabled: powerdns.Bool(false),
			},
		},
	}
}