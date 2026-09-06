package main

import (
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

// newAPIClient is the command boundary for ambient API settings. The API
// package itself does not read environment variables.
func newAPIClient(token string) (*api.Client, error) {
	settings, err := config.APISettingsFromEnv()
	if err != nil {
		return nil, err
	}
	return api.New(token, settings.ClientOptions()...)
}
