package common

import (
	"fmt"
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

type DNSKeyType int
const(
	DNSSecKeyType = iota
	TSIGKeyType
)

type DNSKey struct {
	Name string
	Data string
	Type DNSKeyType
}

func (key DNSKey) String() string {
	return fmt.Sprintf("Name: %s", key.Name)
}