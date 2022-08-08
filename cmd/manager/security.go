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
	"errors"
	"fmt"
	"github.com/Cray-HPE/cray-powerdns-manager/internal/common"
	"github.com/joeig/go-powerdns/v2"
	"io/ioutil"
	"net/http"
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
	var pdnsErr *powerdns.Error
	if err != nil {
		if errors.As(err, &pdnsErr) {
			// If we end up here then some PowerDNS API error occurred, ignore 404 as the key may not exist yet.
			if pdnsErr.StatusCode != http.StatusNotFound {
				return fmt.Errorf("failed to perform TSIG key lookup: %w", pdnsErr)
			}
		} else {
			// More general error case. Connection refused, connection timeout etc.
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
