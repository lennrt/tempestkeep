package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

type observationFetcher interface {
	DeviceObservations(context.Context, int, int64, int64) ([]model.DeviceObs, error)
}

var (
	_ observationFetcher = (*api.Client)(nil)
	_                    = store.Open
	_                    = store.OpenWriter
	_                    = collect.New
	_                    = config.ParseDotenv
)

func TestPublicConstructionCompiles(t *testing.T) {
	client, err := api.New("synthetic-test-token",
		api.WithCacheTTL(0),
		api.WithRequestTimeout(time.Second),
		api.WithRetryPolicy(api.RetryPolicy{MaxAttempts: 1, BaseWait: time.Millisecond, MaxWait: time.Millisecond}),
	)
	if err != nil || client == nil {
		t.Fatalf("api.New: client=%v error=%v", client, err)
	}
	if _, err := config.ParseDotenv([]byte("TEMPEST_CACHE_TTL=0\n")); err != nil {
		t.Fatalf("config.ParseDotenv: %v", err)
	}
	if err := (model.DeviceObs{Epoch: 1}).Validate(); err != nil {
		t.Fatalf("model.DeviceObs.Validate: %v", err)
	}
}
