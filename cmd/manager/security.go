package main

import (
	"fmt"
	"github.com/joeig/go-powerdns/v2"
	"io/ioutil"
	"stash.us.cray.com/CSM/cray-powerdns-manager/internal/common"
	"strings"
)

var (
	DNSSecKeys []common.DNSSECKey
)

func ParseDNSSecKeys() error {
	if *keyDirectory == "" {
		return fmt.Errorf("blank key directory")
	}

	files, err := ioutil.ReadDir(*keyDirectory)
	if err != nil {
		return fmt.Errorf("failed to read key directory: %w", err)
	}

	for _, privateKeyFile := range files {
		if privateKeyFile.IsDir() || strings.HasPrefix(privateKeyFile.Name(), ".") {
			continue
		}

		privateKeyData, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", *keyDirectory, privateKeyFile.Name()))
		if err != nil {
			return fmt.Errorf("failed to read key file: %w", err)
		}

		DNSSecKeys = append(DNSSecKeys, common.DNSSECKey{
			ZoneName:   privateKeyFile.Name(),
			PrivateKey: string(privateKeyData),
		})
	}

	return nil
}

func AddCryptokeyToZone(key common.DNSSECKey) error {
	newCryptokey := powerdns.Cryptokey{
		KeyType:    powerdns.String("csk"),
		Active:     powerdns.Bool(true),
		Privatekey: powerdns.String(key.PrivateKey),
	}

	_, err := pdns.Cryptokeys.Add(key.ZoneName, &newCryptokey)
	if err != nil {
		return fmt.Errorf("failed to add cryptokey to zone: %w", err)
	}

	return nil
}
