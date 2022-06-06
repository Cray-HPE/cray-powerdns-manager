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
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/mitchellh/mapstructure"
	"io/ioutil"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"strings"
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

func getSLSHardware() (hardware []sls_common.GenericHardware, err error) {
	url := fmt.Sprintf("%s/v1/hardware", *slsURL)
	req, err := retryablehttp.NewRequest("GET", url, nil)
	if err != nil {
		err = fmt.Errorf("failed to create new request: %w", err)
		return
	}
	if token != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	req	= req.WithContext(ctx)

	resp, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to do request: %w", err)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &hardware)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal body: %w", err)
	}

	return
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
	req	= req.WithContext(ctx)

	resp, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to do request: %w", err)
		return
	}
	defer resp.Body.Close()

	var originalNetworks []sls_common.Network
	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &originalNetworks)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal body: %w", err)
	}

	// This is a hack to combine networks that really have no business being separate.
	// For example, hmn, hmn_rvr, and hmn_mtn are all the same network. So what we're going to do is check every
	// network to see if it's really a subset of a real top level network. If so, then combine it and remove it.
	ignoredNetworks := make(map[string]sls_common.Network)
	for networkIndex, _ := range originalNetworks {
		network := &originalNetworks[networkIndex]
		subsetString := fmt.Sprintf("%s_", network.Name)

		var parentNetworkProperties NetworkExtraProperties
		err = mapstructure.Decode(network.ExtraPropertiesRaw, &parentNetworkProperties)
		if err != nil {
			return
		}

		for _, subsetNetwork := range originalNetworks {
			if strings.HasPrefix(subsetNetwork.Name, subsetString) {
				// If this network is a subset of any other network it should be ignored.
				ignoredNetworks[subsetNetwork.Name] = subsetNetwork

				// Now combine the two.
				network.IPRanges = append(network.IPRanges, subsetNetwork.IPRanges...)

				var subsetNetworkProperties NetworkExtraProperties
				err = mapstructure.Decode(subsetNetwork.ExtraPropertiesRaw, &subsetNetworkProperties)
				if err != nil {
					return
				}

				parentNetworkProperties.Subnets = append(parentNetworkProperties.Subnets,
					subsetNetworkProperties.Subnets...)
			}
		}

		network.ExtraPropertiesRaw = parentNetworkProperties
	}

	// Now we can return all the networks we're not explicitly ignoring.
	for _, network := range originalNetworks {
		if _, ok := ignoredNetworks[network.Name]; !ok {
			networks = append(networks, network)
		}
	}

	return
}
