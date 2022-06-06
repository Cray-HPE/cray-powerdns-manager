package main

import (
	"encoding/json"
	"fmt"
	"github.com/joeig/go-powerdns/v2"
	"io/ioutil"
	"net/http"
	"github.com/Cray-HPE/cray-powerdns-manager/internal/common"
	"strings"
)

const tsigExtension = ".tsig"

var (
	DNSKeys []common.DNSKey
)

func ParseDNSKeys() error {
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

		var keyType common.DNSKeyType
		keyName := privateKeyFile.Name()

		// Intuit the type from the name. If it has a .tsig extension, assume it to be so.
		if strings.HasSuffix(privateKeyFile.Name(), tsigExtension) {
			keyType = common.TSIGKeyType

			// Whack off this extension for use everywhere else.
			keyName = strings.TrimSuffix(keyName, tsigExtension)
		} else {
			keyType = common.DNSSecKeyType
		}

		DNSKeys = append(DNSKeys, common.DNSKey{
			Name: keyName,
			Data: string(privateKeyData),
			Type: keyType,
		})
	}

	return nil
}

func AddCryptokeyToZone(key common.DNSKey) error {
	newCryptokey := powerdns.Cryptokey{
		KeyType:    powerdns.String("csk"),
		Active:     powerdns.Bool(true),
		Privatekey: powerdns.String(key.Data),
	}

	_, err := pdns.Cryptokeys.Add(key.Name, &newCryptokey)
	if err != nil {
		return fmt.Errorf("failed to add cryptokey to zone: %w", err)
	}

	return nil
}

func AddOrUpdateTSIGKey(key common.DNSKey) error {
	var existingTSIGKey *powerdns.TSIGKey
	var newTSIGKey *powerdns.TSIGKey
	var err error

	// The data in the DNSKey is actually a JSON block that if the user did as instructed can be natively unmarshalled
	// into the PowerDNS struct.
	jsonErr := json.Unmarshal([]byte(key.Data), &newTSIGKey)
	if jsonErr != nil {
		return fmt.Errorf("failed to unmarshal TSIG key: %w", jsonErr)
	}

	// Get any existing key by this name.
	existingTSIGKey, err = pdns.TSIGKeys.Get(key.Name)
	if err != nil {
		pdnsErr := err.(*powerdns.Error)
		if pdnsErr.StatusCode != http.StatusNotFound {
			return fmt.Errorf("failed to perform TSIG key lookup: %w", err)
		}
	}

	// At this point the key either has a value in the structure or it's nil. Check if we need to add or update.
	var addOrUpdateErr error
	if existingTSIGKey.Key != nil {
		if *existingTSIGKey.Key != *newTSIGKey.Key {
			_, addOrUpdateErr = pdns.TSIGKeys.Replace(key.Name, newTSIGKey)
		}
	} else {
		_, addOrUpdateErr = pdns.TSIGKeys.Add(newTSIGKey)
	}

	if addOrUpdateErr != nil {
		return fmt.Errorf("failed to add TSIG key: %w", addOrUpdateErr)
	}

	return nil
}
