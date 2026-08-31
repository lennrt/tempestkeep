package main

import (
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

func newAPIClient(token string) (*api.Client, error) {
	settings, err := config.APISettingsFromEnv()
	if err != nil {
		return nil, err
	}
	options := []api.Option{api.WithCacheTTL(settings.CacheTTL)}
	if settings.BaseURL != "" {
		options = append(options, api.WithBaseURL(settings.BaseURL))
	}
	return api.New(token, options...)
}
