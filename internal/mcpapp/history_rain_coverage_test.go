package mcpapp

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHistoricalRainWireCoverage(t *testing.T) {
	for _, measured := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "measured zero"}[measured], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.sqlite")
			w, e := store.OpenWriter(t.Context(), path)
			if e != nil {
				t.Fatal(e)
			}
			var rain *float64
			if measured {
				rain = new(0.0)
			}
			_, e = w.InsertObs(t.Context(), 456, []model.DeviceObs{{Epoch: time.Date(2024, 1, 15, 12, 0, 0, 0, time.Local).Unix(), RainMm: rain}})
			if e = errors.Join(e, w.Close()); e != nil {
				t.Fatal(e)
			}
			cs := connectArchiveServer(t, t.Context(), path)
			listed, e := cs.ListTools(t.Context(), nil)
			if e != nil {
				t.Fatal(e)
			}
			for _, tc := range []struct {
				name, array, total, count string
				args                      map[string]any
			}{
				{"this_day_in_history", "years", "rain_in", "rain_readings", map[string]any{"date": "01-15"}},
				{"period_summary", "periods", "rain_in", "rain_readings", map[string]any{"period": "month"}},
				{"climate_normals", "months", "avg_rain_in", "rain_years", nil},
			} {
				var schema string
				for _, tool := range listed.Tools {
					if tool.Name == tc.name {
						b, e := json.Marshal(tool.OutputSchema)
						if e != nil {
							t.Fatal(e)
						}
						schema = string(b)
					}
				}
				if !strings.Contains(schema, `"`+tc.count+`"`) || !strings.Contains(schema, `"null"`) {
					t.Fatalf("schema does not expose nullable rain/count: %s", schema)
				}
				result, e := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
				if e != nil || result.IsError {
					t.Fatalf("call %s: %v %+v", tc.name, e, result)
				}
				var structured map[string]any
				decodeStructured(t, result, &structured)
				rows := structured[tc.array].([]any)
				row := rows[0].(map[string]any)
				v, exists := row[tc.total]
				if !exists || !measured && v != nil || measured && v != float64(0) {
					t.Fatalf("%s total/count: %+v", tc.name, row)
				}
				count := float64(0)
				if measured {
					count = 1
				}
				if row[tc.count] != count {
					t.Fatalf("count: %+v", row)
				}
				plain, ok := result.Content[0].(*mcp.TextContent)
				if !ok {
					t.Fatal("missing plain text")
				}
				var decoded map[string]any
				if e = json.Unmarshal([]byte(plain.Text), &decoded); e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(decoded, structured) {
					t.Fatalf("plain and structured differ: %s", plain.Text)
				}
			}
		})
	}
}
