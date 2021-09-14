package powerdns

import (
	"fmt"
)

// TSIGKeysService handles communication with the TSIGKeys related methods of the Client API
type TSIGKeysService service

// TSIGKey structure with JSON API metadata
type TSIGKey struct {
	Name      *string `json:"name,omitempty"`
	ID        *string `json:"id,omitempty"`
	Algorithm *string `json:"algorithm,omitempty"`
	Key       *string `json:"key,omitempty"`
	Type      *string `json:"type,omitempty"`
}

// List retrieves a list of TSIGKeys
func (t *TSIGKeysService) List() ([]TSIGKey, error) {
	req, err := t.client.newRequest("GET",
		fmt.Sprintf("servers/%s/tsigkeys", t.client.VHost), nil, nil)
	if err != nil {
		return nil, err
	}

	TSIGKeys := make([]TSIGKey, 0)
	_, err = t.client.do(req, &TSIGKeys)
	return TSIGKeys, err
}

// Add creates a certain TSIGKey instance
func (t *TSIGKeysService) Add(TSIGKey *TSIGKey) (*TSIGKey, error) {
	req, err := t.client.newRequest("POST",
		fmt.Sprintf("servers/%s/tsigkeys", t.client.VHost), nil, TSIGKey)
	if err != nil {
		return nil, err
	}

	_, err = t.client.do(req, &TSIGKey)
	return TSIGKey, err
}

// Replace replaces a certain TSIGKey instance
func (t *TSIGKeysService) Replace(id string, TSIGKey *TSIGKey) (*TSIGKey, error) {
	req, err := t.client.newRequest("PUT",
		fmt.Sprintf("servers/%s/tsigkeys/%s", t.client.VHost, id), nil, TSIGKey)
	if err != nil {
		return nil, err
	}

	_, err = t.client.do(req, &TSIGKey)
	return TSIGKey, err
}

// Get returns a certain TSIGKey instance
func (t *TSIGKeysService) Get(id string) (*TSIGKey, error) {
	req, err := t.client.newRequest("GET",
		fmt.Sprintf("servers/%s/tsigkeys/%s", t.client.VHost, id), nil, nil)
	if err != nil {
		return nil, err
	}

	TSIGKey := new(TSIGKey)
	_, err = t.client.do(req, &TSIGKey)
	return TSIGKey, err
}

// Delete removes a given TSIGKey
func (t *TSIGKeysService) Delete(id string) error {
	req, err := t.client.newRequest("DELETE",
		fmt.Sprintf("servers/%s/tsigkeys/%s", t.client.VHost, id), nil, nil)
	if err != nil {
		return err
	}

	_, err = t.client.do(req, nil)
	return err
}
