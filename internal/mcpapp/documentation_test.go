package mcpapp

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Read the examples themselves so a documentation edit cannot silently leave
// these checks exercising a stale copy. These guides use simple fenced blocks.
var documentedFence = regexp.MustCompile("(?s)```(json|sql)\\r?\\n(.*?)\\r?\\n```")

func documentationBlocks(t *testing.T, path, language string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	var blocks []string
	for _, match := range documentedFence.FindAllStringSubmatch(string(data), -1) {
		if match[1] == language {
			blocks = append(blocks, match[2])
		}
	}
	if len(blocks) == 0 {
		t.Fatalf("%s has no %s examples", path, language)
	}
	return blocks
}

func TestDocumentationJSONExamples(t *testing.T) {
	for _, path := range []string{
		"README.md", "docs/mcp.md", "docs/querying.md", "plugin/skills/station-analyst/SKILL.md",
	} {
		t.Run(path, func(t *testing.T) {
			for index, block := range documentationBlocks(t, path, "json") {
				if !json.Valid([]byte(block)) {
					t.Errorf("JSON example %d is invalid", index+1)
				}
			}
		})
	}
}

func TestIntegrationDocumentedQueries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "examples.sqlite")
	w, err := store.OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, w)
	epoch := func(value string) int64 {
		at, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return at.Unix()
	}
	// Large rain values just outside June catch inclusive-end and UTC mistakes.
	// The middle row lacks rain; the last June row lacks temperature.
	_, err = w.InsertObs(t.Context(), 7, []model.DeviceObs{
		{Epoch: epoch("2025-05-31T23:59:00Z"), RainMm: new(99.0), AirTempC: new(10.0)},
		{Epoch: epoch("2025-06-01T00:00:00Z"), RainMm: new(2.54), AirTempC: new(20.0), WindAvg: new(2.0), WindGust: new(4.0)},
		{Epoch: epoch("2025-06-03T12:00:00Z"), AirTempC: new(25.0), WindAvg: new(4.0), WindGust: new(12.0)},
		{Epoch: epoch("2025-06-30T23:59:00Z"), RainMm: new(5.08), WindAvg: new(3.0), WindGust: new(8.0)},
		{Epoch: epoch("2025-07-01T00:00:00Z"), RainMm: new(99.0), AirTempC: new(30.0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	session := connectArchiveServer(t, t.Context(), path)
	call := func(t *testing.T, name string, args any) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("documented %s call failed: %v", name, result.Content)
		}
		return result
	}

	t.Run("tool argument examples", func(t *testing.T) {
		examples := documentationBlocks(t, "docs/querying.md", "json")
		tools := []string{"daily_summary", "get_observations", "period_summary", "period_summary", "query_sql"}
		if len(examples) != len(tools) {
			t.Fatalf("guide has %d calls, test maps %d: map new examples to their tool", len(examples), len(tools))
		}
		for index, block := range examples {
			var args map[string]any
			if err := json.Unmarshal([]byte(block), &args); err != nil {
				t.Fatal(err)
			}
			result := call(t, tools[index], args)
			if tools[index] == "query_sql" {
				var got store.QueryResult
				decodeStructured(t, result, &got)
				if got.RowCount != 4 || got.Rows[0][1] != float64(30) {
					t.Errorf("recent temperature example = %+v, want four rows in descending order and Celsius", got)
				}
			}
		}
	})

	sqlExamples := documentationBlocks(t, "docs/querying.md", "sql")
	if len(sqlExamples) != 2 {
		t.Fatalf("guide has %d SQL examples, want both coverage and gust examples", len(sqlExamples))
	}
	t.Run("rain coverage and UTC bounds", func(t *testing.T) {
		result := call(t, "query_sql", map[string]any{"sql": sqlExamples[0], "max_rows": 10})
		var got store.QueryResult
		decodeStructured(t, result, &got)
		if got.RowCount != 1 || len(got.Rows[0]) != 4 {
			t.Fatalf("rain example shape = %+v", got)
		}
		for column, want := range []float64{3, 2, 7.62, 0.3} {
			value, ok := got.Rows[0][column].(float64)
			if !ok || math.Abs(value-want) > 1e-9 {
				t.Errorf("rain column %d = %v, want %v", column, got.Rows[0][column], want)
			}
		}
	})
	t.Run("gust ranking and units", func(t *testing.T) {
		result := call(t, "query_sql", map[string]any{"sql": sqlExamples[1], "max_rows": 10})
		var got store.QueryResult
		decodeStructured(t, result, &got)
		if got.RowCount != 3 || len(got.Rows[0]) != 5 {
			t.Fatalf("gust example shape = %+v", got)
		}
		want := []any{"2025-06-03", float64(26.8), float64(8.9), float64(1), float64(1)}
		for column, value := range want {
			if got.Rows[0][column] != value {
				t.Errorf("gust column %d = %v, want %v", column, got.Rows[0][column], value)
			}
		}
	})
}
