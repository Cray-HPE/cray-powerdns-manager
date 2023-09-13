package main

import (
	"fmt"
	base "github.com/Cray-HPE/hms-base"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/mitchellh/mapstructure"
	"regexp"
	"strconv"
)

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
				err = fmt.Errorf("Unable to decode node ExtraProperties")
				return
			} else {
				// Try SLS first
				if extraProperties.NID != 0 {
					hasNid = true
					hostname = fmt.Sprintf("%s%06d", *nidPrefix, extraProperties.NID)
					nic, _ = strconv.Atoi(matches[re.SubexpIndex("Nic")])
				} else {
					// Application nodes have the NID assigned by SMD, try there.
					nodeState, found := stateMap[matches[xname]]
					if found {
						nid, err := nodeState.NID.Int64()
						if err == nil {
							hasNid = true
							hostname = fmt.Sprintf("%s%d", *nidPrefix, nid)
							nic, _ = strconv.Atoi(matches[re.SubexpIndex("Nic")])
						}
					}

				}
				if hasNid == false {
					err = fmt.Errorf("Unable to find NID for node %s in SLS hardware map", reservation)
					return
				}
			}
		} else {
			// Cannot find record in SLS hardware map
			err = fmt.Errorf("Unable to find node %s in SLS hardware map", reservation)
			return
		}
	} else {
		// SLS IPReservation xname not in correct format
		err = fmt.Errorf("name %s not in correct format", reservation)
		return
	}
	return
}
