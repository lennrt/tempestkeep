package mcpapp

import (
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

// newAPIClient backs the Options.Token fallback by reading the ambient
// TEMPEST_API_* settings. Callers that already own a client should set
// Options.Client instead, so this package never has to read the environment.
func newAPIClient(token string) (*api.Client, error) {
	settings, err := config.APISettingsFromEnv()
	if err != nil {
		return nil, err
	}
	return api.New(token, settings.ClientOptions()...)
}
