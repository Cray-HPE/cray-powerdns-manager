package main

import (
	"fmt"
	sls_common "github.com/Cray-HPE/hms-sls/pkg/sls-common"
	"github.com/mitchellh/mapstructure"
	"regexp"
	"strconv"
)

func getHSNNidNic(reservation string,
	hardwareMap map[string]sls_common.GenericHardware) (hostname string, nic int, err error) {

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
				// No NID found
				if extraProperties.NID == 0 {
					err = fmt.Errorf("Unable to find NID for node %s in SLS hardware map", reservation)
					return
				}
				hostname = fmt.Sprintf("%s%06d", *nidPrefix, extraProperties.NID)
				nic, _ = strconv.Atoi(matches[re.SubexpIndex("Nic")])
			}
		} else {
			// Cannot find record in SLS hardware map
			err = fmt.Errorf("Unable to find node %s in SLS hardware map", reservation)
			return
		}
	} else {
		// SLS IPReservation xname not in correct format
		err = fmt.Errorf("name %s not in correct format",reservation)
		return
	}
	return
}
