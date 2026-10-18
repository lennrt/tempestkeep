package mcpapp

import (
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRainStatsSchemaAndCoverage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rain     []*float64
		readings int
		total    *float64
		note     string
	}{
		{"empty", nil, 0, nil, "no observations"},
		{"missing", []*float64{nil, nil}, 0, nil, "no rain readings"},
		{"measured zero", []*float64{new(0.0), new(0.0)}, 2, new(0.0), ""},
		{"partial zero", []*float64{new(0.0), nil}, 1, new(0.0), "partial rain coverage"},
		{"partial positive", []*float64{new(25.4), nil}, 1, new(1.0), "partial rain coverage"},
		{"reported positive", []*float64{new(25.4), new(12.7)}, 2, new(1.5), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			writer, err := store.OpenWriter(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			base := time.Date(2025, time.June, 1, 12, 0, 0, 0, time.Local)
			rows := []model.DeviceObs{
				{Epoch: base.AddDate(0, 0, -1).Unix(), RainMm: new(100.0)},
				{Epoch: base.AddDate(0, 0, 1).Unix(), RainMm: new(100.0)},
			}
			for i, rain := range tc.rain {
				rows = append(rows, model.DeviceObs{Epoch: base.Unix() + int64(i*60), RainMm: rain})
			}
			_, insertErr := writer.InsertObs(t.Context(), 456, rows)
			if err := errors.Join(insertErr, writer.Close()); err != nil {
				t.Fatal(err)
			}
			cs := connectArchiveServer(t, t.Context(), path)
			listed, err := cs.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, tool := range listed.Tools {
				if tool.Name != "rain_stats" {
					continue
				}
				found = true
				encoded, err := json.Marshal(tool.OutputSchema)
				if err != nil {
					t.Fatal(err)
				}
				var schema struct {
					Properties map[string]json.RawMessage `json:"properties"`
					Required   []string                   `json:"required"`
				}
				if err := json.Unmarshal(encoded, &schema); err != nil {
					t.Fatal(err)
				}
				var totalSchema struct {
					Types []string `json:"type"`
				}
				if err := json.Unmarshal(schema.Properties["total_in"], &totalSchema); err != nil {
					t.Fatalf("nullable total schema: %s: %v", encoded, err)
				}
				if !slices.Contains(totalSchema.Types, "null") || !slices.Contains(totalSchema.Types, "number") {
					t.Fatalf("total schema must permit number and null: %s", encoded)
				}
				for _, name := range []string{"total_in", "observations", "rain_readings"} {
					if !slices.Contains(schema.Required, name) {
						t.Fatalf("%s missing from required schema fields: %s", name, encoded)
					}
				}
			}
			if !found {
				t.Fatal("rain_stats tool absent")
			}
			result, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "rain_stats", Arguments: map[string]any{"start": "2025-06-01", "end": "2025-06-01"}})
			if err != nil || result.IsError {
				t.Fatalf("rain call: error=%v result=%+v", err, result)
			}
			var structured map[string]any
			decodeStructured(t, result, &structured)
			value, exists := structured["total_in"]
			if !exists || (tc.total == nil && value != nil) {
				t.Fatalf("missing total must be explicit null: %+v", structured)
			}
			if tc.total != nil {
				got, numeric := value.(float64)
				if !numeric || math.Abs(got-*tc.total) > 1e-9 {
					t.Fatalf("total=%v, want%v", value, *tc.total)
				}
			}
			if structured["observations"] != float64(len(tc.rain)) || structured["rain_readings"] != float64(tc.readings) {
				t.Fatalf("wrong range sample counts: %+v", structured)
			}
			note, _ := structured["note"].(string)
			if tc.note == "" && note != "" || tc.note != "" && !strings.Contains(note, tc.note) {
				t.Fatalf("coverage note=%q, want%q", note, tc.note)
			}
			if len(result.Content) != 1 {
				t.Fatalf("expected one ordinary result: %+v", result.Content)
			}
			plain, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatal("ordinary MCP content is not text")
			}
			var textResult map[string]any
			if err := json.Unmarshal([]byte(plain.Text), &textResult); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(structured, textResult) {
				t.Fatalf("ordinary/structured rain results differ: %s / %+v", plain.Text, structured)
			}
		})
	}
}
