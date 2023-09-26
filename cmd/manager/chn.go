package main

import (
	"fmt"
	base "github.com/Cray-HPE/hms-base"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/mitchellh/mapstructure"
	"regexp"
	"strconv"
)

// getHSNNidNic returns the NID alias (e.g. nid001000) for a given xname
// and the HSN NIC number or an error if the NID cannot be determined.
//
// SLS is tried first however Application nodes are a special case as the
// NID is assigned by SMD when the node is discovered.
func getHSNNidNic(reservation string,
	hardwareMap map[string]sls_common.GenericHardware,
	stateMap map[string]base.Component) (hostname string, nic int, err error) {

	var hasNid bool = false

	re := regexp.MustCompile(`(?m)(?P<Xname>^x.*)h(?P<Nic>\d+)`)
	matches := re.FindStringSubmatch(reservation)

	if matches != nil {
		xname := re.SubexpIndex("Xname")
		node, found := hardwareMap[matches[xname]]

		if found {
			var extraProperties sls_common.ComptypeNode
			e := mapstructure.Decode(node.ExtraPropertiesRaw, &extraProperties)
			if e != nil {
				err = fmt.Errorf("unable to decode node ExtraProperties")
				return
			} else {
				// Try SLS first
				if extraProperties.NID != 0 {
					hasNid = true
					// TODO: Make the zero padding configurable.
					hostname = fmt.Sprintf("%s%06d", *nidPrefix, extraProperties.NID)
					nic, _ = strconv.Atoi(matches[re.SubexpIndex("Nic")])
				} else {
					// Application nodes have the NID assigned by SMD, try there.
					nodeState, foundSMD := stateMap[matches[xname]]
					if foundSMD {
						nid, err := nodeState.NID.Int64()
						if err == nil {
							hasNid = true
							hostname = fmt.Sprintf("%s%d", *nidPrefix, nid)
							nic, _ = strconv.Atoi(matches[re.SubexpIndex("Nic")])
						}
					}

				}
				// Unable to find a NID in SLS or SMD
				if hasNid == false {
					err = fmt.Errorf("unable to find NID in SMD or SLS for node %s", reservation)
					return
				}
			}
		} else {
			// Cannot find record in SLS hardware map
			err = fmt.Errorf("unable to find node %s in SLS hardware map", reservation)
			return
		}
	} else {
		// SLS IPReservation xname not in correct format
		err = fmt.Errorf("name %s not in correct format", reservation)
		return
	}
	return
}
