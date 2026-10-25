package main

import (
	"encoding/json"

	"github.com/akashprasad/file-uploader/packages/sdk"
	"github.com/zalando/go-keyring"
)

const keychainService = "file-uploader"

func loadProfile(provider sdk.Provider) (map[string]string, error) {
	data, err := keyring.Get(keychainService, string(provider))
	if err != nil {
		return nil, err
	}
	var profile map[string]string
	if err := json.Unmarshal([]byte(data), &profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func saveProfile(provider sdk.Provider, profile map[string]string) error {
	data, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return keyring.Set(keychainService, string(provider), string(data))
}
