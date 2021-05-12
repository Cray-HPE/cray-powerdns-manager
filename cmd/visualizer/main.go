package main

import (
	"crypto/tls"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/joeig/go-powerdns/v2"
	"github.com/namsral/flag"
	"github.com/xlab/treeprint"
	"net/http"
	"time"
)

var (
	pdnsURL = flag.String("pdns_url", "http://localhost:9090", "PowerDNS URL")
	pdnsAPIKey = flag.String("pdns_api_key", "cray", "PowerDNS API Key")

	pdns *powerdns.Client

	httpClient *retryablehttp.Client
)

func getCNAMEsForRRset(aName string, rrSets []powerdns.RRset) (cnames []string) {
	for _, rrSet := range rrSets {
		if *rrSet.Type == powerdns.RRTypeCNAME {
			for _, record := range rrSet.Records {
				if *record.Content == aName {
					cnames = append(cnames, *rrSet.Name)
				}
			}
		}
	}

	return
}

func main() {
	// Parse the arguments.
	flag.Parse()

	// For performance reasons we'll keep the client that was created for this base request and reuse it later.
	httpClient = retryablehttp.NewClient()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient.HTTPClient.Transport = transport

	httpClient.RetryMax = 3
	httpClient.RetryWaitMax = time.Second * 2

	// Setup the PowerDNS configuration.
	pdns = powerdns.NewClient(*pdnsURL, "localhost", map[string]string{"X-API-Key": *pdnsAPIKey},
		httpClient.HTTPClient)

	// Get all the Zones.
	zones, err := pdns.Zones.List()
	if err != nil {
		panic(err)
	}

	tree := treeprint.New()

	for _, zone := range zones {
		zone, err := pdns.Zones.Get(*zone.Name)
		if err != nil {
			panic(err)
		}

		zoneBranch := tree.AddBranch(*zone.Name)

		for _, rrSet := range zone.RRsets {
			if *rrSet.Type == powerdns.RRTypeA || *rrSet.Type == powerdns.RRTypePTR {
				nodeBranch := zoneBranch.AddBranch(*rrSet.Name)

				cnames := getCNAMEsForRRset(*rrSet.Name, zone.RRsets)
				for _, cname := range cnames {
					nodeBranch.AddNode(cname)
				}
			}
		}
	}

	fmt.Println(tree.String())
}
