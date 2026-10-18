package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func runExport(t *testing.T, args ...string) (string, error) {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "export.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()
	previous := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = previous }()
	exportErr := cmdExport(args)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), exportErr
}

func exportMetadataArchive(t *testing.T) (string, int64, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.sqlite")
	base := time.Date(2025, time.June, 1, 12, 0, 0, 0, time.Local).Unix()
	w, err := store.OpenWriter(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.InsertObs(t.Context(), 456, []model.DeviceObs{
		{Epoch: base, SolarWm2: new(100.0), ReportIntervalMin: new(1.0), WindInterval: new(3.0), PrecipType: new(2.0)},
		{Epoch: base + 60, SolarWm2: new(200.0), ReportIntervalMin: new(0.0), WindInterval: new(0.0), PrecipType: new(0.0)},
		{Epoch: base + 120, SolarWm2: new(300.0)},
		{Epoch: base + 180, ReportIntervalMin: new(1.23456789), WindInterval: new(2.3456789)},
	}); err != nil {
		t.Fatal(errors.Join(err, w.Close()))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	source := "synthetic \"source\", with\nnewline\tand control \x00\x01 and snow 雪"
	if _, err := db.ExecContext(t.Context(), "UPDATE obs_st SET source=? WHERE epoch=?", source, base+60); err != nil {
		t.Fatal(errors.Join(err, db.Close()))
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE obs_st SET source=NULL WHERE epoch=?", base+120); err != nil {
		t.Fatal(errors.Join(err, db.Close()))
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path, base, source
}

func TestExportMetadataReproducesSolarEstimate(t *testing.T) {
	t.Chdir(t.TempDir())
	path, base, source := exportMetadataArchive(t)
	metadataKeys := []string{"device_id", "report_interval_min", "wind_interval_s", "precip_type", "source"}
	for _, format := range []string{"csv", "jsonl"} {
		for _, units := range []string{"si", "us"} {
			t.Run(format+"/"+units, func(t *testing.T) {
				args := []string{"--db", path, "--format", format, "--units", units, "--start", "2025-06-01", "--end", "2025-06-01"}
				legacy, err := runExport(t, args...)
				if err != nil {
					t.Fatal(err)
				}
				output, err := runExport(t, append(args, "--metadata")...)
				if err != nil {
					t.Fatal(err)
				}
				var rows []map[string]string
				if format == "csv" {
					records, err := csv.NewReader(strings.NewReader(output)).ReadAll()
					if err != nil {
						t.Fatal(err)
					}
					original, err := csv.NewReader(strings.NewReader(legacy)).ReadAll()
					if err != nil {
						t.Fatal(err)
					}
					if len(records) != 5 || len(original) != len(records) || strings.Join(records[0][len(original[0]):], ",") != strings.Join(metadataKeys, ",") {
						t.Fatalf("unexpected metadata CSV header or rows: %q", records)
					}
					for i, record := range records {
						if strings.Join(record[:len(original[i])], "\x00") != strings.Join(original[i], "\x00") {
							t.Fatalf("metadata changed original CSV fields in row %d", i)
						}
						if i == 0 {
							continue
						}
						row := map[string]string{}
						for col, key := range records[0] {
							row[key] = record[col]
						}
						rows = append(rows, row)
					}
				} else {
					original := strings.Split(strings.TrimSpace(legacy), "\n")
					scanner := bufio.NewScanner(strings.NewReader(output))
					for scanner.Scan() {
						var values map[string]json.RawMessage
						if err := json.Unmarshal(scanner.Bytes(), &values); err != nil {
							t.Fatal(err)
						}
						row := map[string]string{}
						for key, raw := range values {
							if key == "time" || key == "source" {
								var value string
								if err := json.Unmarshal(raw, &value); err != nil {
									t.Fatal(err)
								}
								row[key] = value
							} else {
								row[key] = string(raw)
							}
						}
						var originalFields map[string]json.RawMessage
						if err := json.Unmarshal([]byte(original[len(rows)]), &originalFields); err != nil {
							t.Fatal(err)
						}
						for key, want := range originalFields {
							if string(values[key]) != string(want) {
								t.Fatalf("metadata changed original JSON field %s", key)
							}
						}
						rows = append(rows, row)
					}
					if err := scanner.Err(); err != nil {
						t.Fatal(err)
					}
				}
				if len(rows) != 4 {
					t.Fatalf("got %d rows, want 4", len(rows))
				}
				var energy float64
				for i, row := range rows {
					if row["epoch"] != strconv.FormatInt(base+int64(i)*60, 10) || row["device_id"] != "456" {
						t.Fatalf("row identity/order lost: %v", row)
					}
					interval, _ := strconv.ParseFloat(row["report_interval_min"], 64)
					solar, _ := strconv.ParseFloat(row["solar_wm2"], 64)
					if interval > 0 {
						energy += solar * interval * 60 / 1e6
					}
				}
				if math.Abs(energy-.006) > 1e-12 {
					t.Fatalf("export-derived solar energy = %g, want 0.006 MJ/m²", energy)
				}
				if rows[0]["report_interval_min"] != "1" || rows[0]["wind_interval_s"] != "3" || rows[0]["precip_type"] != "2" || rows[0]["source"] != "rest" {
					t.Fatalf("stored metadata was lost: %v", rows[0])
				}
				if rows[1]["report_interval_min"] != "0" || rows[1]["wind_interval_s"] != "0" || rows[1]["precip_type"] != "0" || rows[1]["source"] != source {
					t.Fatalf("zero metadata or source escaping lost: %v", rows[1])
				}
				for _, key := range metadataKeys[1:] {
					value, present := rows[2][key]
					if value != "" || format == "jsonl" && present {
						t.Errorf("missing %s became %q (present=%v)", key, value, present)
					}
				}
				if rows[3]["report_interval_min"] != "1.23456789" || rows[3]["wind_interval_s"] != "2.3456789" {
					t.Fatalf("metadata precision changed under %s: %v", units, rows[3])
				}
			})
		}
	}
}

func TestDefaultExportShapeRemainsStable(t *testing.T) {
	t.Chdir(t.TempDir())
	path, base, _ := exportMetadataArchive(t)
	for _, format := range []string{"csv", "jsonl"} {
		output, err := runExport(t, "--db", path, "--format", format)
		if err != nil {
			t.Fatal(err)
		}
		if format == "csv" {
			want := "epoch,time,wind_lull_mps,wind_avg_mps,wind_gust_mps,wind_dir_deg,pressure_mb,air_temp_c,humidity_pct,illuminance_lux,uv,solar_wm2,rain_mm,strike_dist_km,strike_count,battery_v\n"
			if !strings.HasPrefix(output, want) {
				t.Fatalf("default CSV header changed: %s", output)
			}
		} else {
			want := fmt.Sprintf("{\"epoch\":%d,\"time\":%q,\"solar_wm2\":100}\n", base, time.Unix(base, 0).Local().Format(time.RFC3339))
			if !strings.HasPrefix(output, want) {
				t.Fatalf("default JSON output changed: %s", output)
			}
		}
		for _, key := range []string{"device_id", "report_interval_min", "wind_interval_s", "precip_type", "source"} {
			if strings.Contains(output, key) {
				t.Errorf("default %s unexpectedly contains %s", format, key)
			}
		}
	}
}

func TestExportMetadataOutputFailureAndCancellation(t *testing.T) {
	path, base, _ := exportMetadataArchive(t)
	s, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	closeOnCleanup(t, s)
	for _, write := range []struct {
		name string
		fn   func(context.Context, *store.Store, *bufio.Writer, int64, int64, bool, bool) error
	}{{"csv", exportCSV}, {"jsonl", exportJSONL}} {
		t.Run(write.name, func(t *testing.T) {
			out := bufio.NewWriterSize(failedOutput{}, 1)
			if err := write.fn(t.Context(), s, out, base, base+180, false, true); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("output failure = %v, want closed pipe", err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			out = bufio.NewWriter(io.Discard)
			if err := write.fn(ctx, s, out, base, base+180, false, true); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled export = %v", err)
			}
			checkCtx, stop := context.WithTimeout(t.Context(), time.Second)
			defer stop()
			if _, err := s.Coverage(checkCtx); err != nil {
				t.Fatalf("output failure/cancellation leaked rows: %v", err)
			}
		})
	}
}
