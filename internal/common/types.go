package common

import (
	"github.com/joeig/go-powerdns/v2"
	"net"
)

const rdnsDomain = ".in-addr.arpa"

type Nameserver struct {
	FQDN string
	IP   string
}

type NetworkNameCIDRMap struct {
	Name string
	CIDR *net.IPNet
}

type PowerDNSZones []*powerdns.Zone

type DNSSECKey struct {
	ZoneName string
	PrivateKey string
}