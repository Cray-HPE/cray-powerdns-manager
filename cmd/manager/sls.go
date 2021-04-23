package main

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"io/ioutil"
	sls_common "stash.us.cray.com/HMS/hms-sls/pkg/sls-common"
)

// It's a shame to have to do this, but, because SLS native structures use the IP type which internally is an array of
// bytes we need a more vanilla structure to allow us to work with that data. In truth this kind of feels like a bug to
// me. For some reason when mapstructure is using the reflect package to get the `Kind()` of those data defined as
// net.IP it's giving back slice instead of string.

// NetworkExtraProperties provides additional network information
type NetworkExtraProperties struct {
	CIDR      string  `json:"CIDR"`
	VlanRange []int16 `json:"VlanRange"`
	MTU       int16   `json:"MTU,omitempty"`
	Comment   string  `json:"Comment,omitempty"`

	Subnets []IPV4Subnet `json:"Subnets"`
}

// IPReservation is a type for managing IP Reservations
type IPReservation struct {
	Name      string   `json:"Name"`
	IPAddress string   `json:"IPAddress"`
	Aliases   []string `json:"Aliases,omitempty"`

	Comment string `json:"Comment,omitempty"`
}

// IPV4Subnet is a type for managing IPv4 Subnets
type IPV4Subnet struct {
	FullName       string          `json:"FullName"`
	CIDR           string          `json:"CIDR"`
	IPReservations []IPReservation `json:"IPReservations,omitempty"`
	Name           string          `json:"Name"`
	VlanID         int16           `json:"VlanID"`
	Gateway        string          `json:"Gateway"`
	DHCPStart      string          `json:"DHCPStart,omitempty"`
	DHCPEnd        string          `json:"DHCPEnd,omitempty"`
	Comment        string          `json:"Comment,omitempty"`
}

func getSLSNetworks() (networks []sls_common.Network, err error) {
	url := fmt.Sprintf("%s/v1/networks",
		*slsURL)
	req, err := retryablehttp.NewRequest("GET", url, nil)
	if err != nil {
		err = fmt.Errorf("failed to create new request: %w", err)
		return
	}
	if token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to do request: %w", err)
		return
	}

	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &networks)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal body: %w", err)
	}

	return
}
