package main

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"io/ioutil"
	"stash.us.cray.com/HMS/hms-smd/pkg/sm"

)

func getHSMEthernetInterfaces() (ethernetInterfaces []sm.CompEthInterface, err error) {
	url := fmt.Sprintf("%s/v1/Inventory/EthernetInterfaces", *hsmURL)
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
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &ethernetInterfaces)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal body: %w", err)
	}

	return
}