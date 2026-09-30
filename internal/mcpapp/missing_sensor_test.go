package mcpapp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestLightningZeroDoesNotClaimCompleteDetection(t *testing.T) {
	for _, count := range []*float64{nil, new(0.0)} {
		name := "missing"
		wantDays := int64(0)
		if count != nil {
			name, wantDays = "measured zero", 1
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := writer.InsertObs(t.Context(), 456, []model.DeviceObs{{Epoch: 1700000000, StrikeCount: count}}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			cs := connectArchiveServer(t, t.Context(), path)
			var out LightningOut
			callTool(t, t.Context(), cs, "lightning_activity", nil, &out)
			if strings.Contains(out.Note, "no lightning detected") || !strings.Contains(out.Note, "no nonzero strike counts") || !strings.Contains(out.Note, "missing strike-count readings") || out.LongestStormFreeDays != wantDays {
				t.Fatalf("lightning coverage was overstated: %+v", out)
			}
		})
	}
}
