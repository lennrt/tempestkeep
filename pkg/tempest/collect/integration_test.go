package collect_test

// Integration test across the api → collect → store seam with only the network
// boundary faked: a real api.Client (pointed at an httptest server via
// TEMPEST_API_BASE) feeds a real store.Writer through the Backfiller. It proves
// the pieces fit together and that a chunked backfill stores exactly the API's
// observations, without the MCP layer on top.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// hourlyObsServer serves one observation per hour within [bandStart, bandEnd],
// intersected with the requested window, a stand-in for the device endpoint.
func hourlyObsServer(t *testing.T, bandStart, bandEnd int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasPrefix(r.URL.Path, "/observations/device/") {
			http.NotFound(w, r)
			return
		}
		start, _ := strconv.ParseInt(r.URL.Query().Get("time_start"), 10, 64)
		end, _ := strconv.ParseInt(r.URL.Query().Get("time_end"), 10, 64)
		if start < bandStart {
			start = bandStart
		}
		if end > bandEnd {
			end = bandEnd
		}
		var obs [][]any
		for e := ((start + 3599) / 3600) * 3600; e <= end; e += 3600 {
			row := make([]any, 18)
			row[0] = e
			row[7] = 18.0 // air_temp_c
			obs = append(obs, row)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"obs": obs})
	}))
}

func TestIntegrationBackfillThroughRealClientAndWriter(t *testing.T) {
	ctx := t.Context()
	const bandStart, bandEnd = 1_700_000_000, 1_700_000_000 + 3*86400 // 3 days
	srv := hourlyObsServer(t, bandStart, bandEnd)
	t.Cleanup(srv.Close)

	client, err := api.New("tok", api.WithBaseURL(srv.URL), api.WithCacheTTL(0))
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.OpenWriter(context.Background(), filepath.Join(t.TempDir(), "integration.sqlite"))
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	closeOnCleanup(t, w)

	// Force multi-chunk walking with a 1-day chunk so the 3-day range takes >1 request.
	bf, err := collect.New(client, w, 456, collect.WithChunkSeconds(86400))
	if err != nil {
		t.Fatal(err)
	}
	res, err := bf.BackfillRange(ctx, bandStart, bandEnd, 0)
	if err != nil {
		t.Fatalf("BackfillRange: %v", err)
	}
	if !res.Done {
		t.Errorf("Done = false, want true")
	}

	// Expected count mirrors the mock's own on-the-hour generation, so it stays
	// correct regardless of where the band edges fall relative to the hour.
	var want int64
	for e := ((int64(bandStart) + 3599) / 3600) * 3600; e <= bandEnd; e += 3600 {
		want++
	}
	cov, err := w.Coverage(ctx, 456)
	if err != nil {
		t.Fatal(err)
	}
	if cov.Count != want {
		t.Errorf("stored rows = %d, want %d", cov.Count, want)
	}
	if int64(res.RowsAdded) != want {
		t.Errorf("RowsAdded = %d, want %d", res.RowsAdded, want)
	}

	// Idempotent: Collect now sees a watermark and syncs forward, adding nothing
	// new (the band's newest hour is already stored and "now" is bandEnd).
	cr, err := bf.Collect(ctx, bandEnd, 0)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if cr.Mode != "incremental" || cr.RowsAdded != 0 {
		t.Errorf("Collect = %+v, want incremental with 0 new rows", cr)
	}
}
