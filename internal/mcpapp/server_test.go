package mcpapp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run(canceled) = %v, want context.Canceled", err)
	}
}

func TestRunRequiresDataSource(t *testing.T) {
	err := Run(t.Context(), Options{})
	if err == nil || !strings.Contains(err.Error(), "no data source") {
		t.Fatalf("Run(no source) = %v, want bounded data-source error", err)
	}
}
