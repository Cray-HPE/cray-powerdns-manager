package common

import "net"

const rdnsDomain = ".in-addr.arpa"

type Nameserver struct {
	FQDN string
	IP   string
}

type NetworkNameCIDRMap struct {
	Name string
	CIDR *net.IPNet
}
