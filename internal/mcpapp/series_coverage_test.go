package mcpapp

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestObservationSeriesReportsEventCoverage(t *testing.T) {
	for _, complete := range []bool{false, true} {
		name := "missing and partial"
		if complete {
			name = "measured zero"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			})
			base := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.Local).Unix()
			observations := []model.DeviceObs{
				{Epoch: base, AirTempC: new(10.0)},
				{Epoch: base + 300, RainMm: new(0.0), StrikeCount: new(0.0)},
				{Epoch: base + 360, RainMm: new(2.54)},
			}
			if complete {
				for i := range observations {
					observations[i].RainMm = new(0.0)
					observations[i].StrikeCount = new(0.0)
				}
			}
			if _, err := writer.InsertObs(t.Context(), 456, observations); err != nil {
				t.Fatal(err)
			}
			cs := connectArchiveServer(t, t.Context(), path)
			var out GetObservationsOut
			callTool(t, t.Context(), cs, "get_observations", map[string]any{
				"start": "2024-01-01", "end": "2024-01-01", "bucket_minutes": 5,
			}, &out)
			if len(out.Points) != 2 {
				t.Fatalf("points = %+v, want 2", out.Points)
			}
			first, second := out.Points[0], out.Points[1]
			if complete {
				if first.RainReadings != 1 || first.LightningReadings != 1 || second.RainReadings != 2 || second.LightningReadings != 2 || out.Note != "" {
					t.Fatalf("complete zero readings lost coverage: %+v", out)
				}
			} else if first.RainReadings != 0 || first.LightningReadings != 0 || second.RainReadings != 2 || second.LightningReadings != 1 || second.Obs != 2 || !strings.Contains(out.Note, "incomplete rain or lightning coverage") {
				t.Fatalf("missing/partial readings lost coverage: %+v", out)
			}
			if first.RainIn != 0 || first.Strikes != 0 || second.Strikes != 0 {
				t.Fatalf("numeric zero compatibility lost: %+v", out)
			}
			if !complete && second.RainIn != model.MmToInch(2.54) {
				t.Fatalf("available rain total = %v, want %v", second.RainIn, model.MmToInch(2.54))
			}
			if !complete {
				callTool(t, t.Context(), cs, "get_observations", map[string]any{
					"start": "2024-01-01", "end": "2024-01-03", "bucket_minutes": 1,
				}, &out)
				if !strings.Contains(out.Note, "requested bucket_minutes was too fine") || !strings.Contains(out.Note, "incomplete rain or lightning coverage") {
					t.Fatalf("resolution and coverage notices must coexist: %q", out.Note)
				}
			}
		})
	}
}
