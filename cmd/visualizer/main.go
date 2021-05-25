package main

import (
	"crypto/tls"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/joeig/go-powerdns/v2"
	"github.com/namsral/flag"
	"github.com/xlab/treeprint"
	"net/http"
	"sort"
	"strings"
	"time"
)

const rdnsDomain = ".in-addr.arpa"

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

	authoratativeRecords := make(map[string][]string)

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

		var thisZoneRecords []string
		zoneBranch := tree.AddBranch(*zone.Name)

		for _, rrSet := range zone.RRsets {
			if *rrSet.Type != powerdns.RRTypeCNAME && *rrSet.Type != powerdns.RRTypeSOA {
				thisZoneRecords = append(thisZoneRecords, *rrSet.Name)
				nodeBranch := zoneBranch.AddMetaBranch(*rrSet.Type, *rrSet.Name)

				if *rrSet.Type == powerdns.RRTypeA || *rrSet.Type == powerdns.RRTypePTR {
					cnames := getCNAMEsForRRset(*rrSet.Name, zone.RRsets)
					for _, cname := range cnames {
						nodeBranch.AddMetaNode(powerdns.RRTypeCNAME, cname)
					}
				}

				if *rrSet.Type == powerdns.RRTypeNS || *rrSet.Type == powerdns.RRTypePTR {
					for _, record := range rrSet.Records {
						nodeBranch.AddNode(*record.Content)
					}
				}
			}
		}

		if !strings.Contains(*zone.Name, rdnsDomain) {
			authoratativeRecords[*zone.Name] = thisZoneRecords
		}
	}

	fmt.Println(tree.String())

	for zone, records := range authoratativeRecords {
		fmt.Printf("%s\n", zone)

		sort.Strings(records)

		for _, record := range records {
			fmt.Printf("\t%s\n", record)
		}
	}
}
