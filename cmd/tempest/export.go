package main

// Command tempest export writes archive rows as CSV or JSON Lines. It scans a
// read-only store and does not buffer the full range. Stored values use SI units.
// --units us converts values during export.

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// exportField is one sensor column. A nil value becomes an empty CSV cell or an
// omitted JSON field.
type exportField struct {
	Name string
	Val  *float64
}

// obsFields returns sensor columns in a stable order. If us is true, it converts
// values to US display units.
func obsFields(o model.Obs, us bool) []exportField {
	conv := func(p *float64, f func(float64) float64) *float64 {
		if p == nil {
			return nil
		}
		v := f(*p)
		return &v
	}
	id := func(v float64) float64 { return v }
	if us {
		return []exportField{
			{"wind_lull_mph", conv(o.WindLullMps, model.MpsToMph)},
			{"wind_avg_mph", conv(o.WindAvgMps, model.MpsToMph)},
			{"wind_gust_mph", conv(o.WindGustMps, model.MpsToMph)},
			{"wind_dir_deg", o.WindDirDeg},
			{"pressure_inhg", conv(o.PressureMb, model.MbToInHg)},
			{"air_temp_f", conv(o.AirTempC, model.CToF)},
			{"humidity_pct", o.HumidityPct},
			{"illuminance_lux", o.IlluminanceLux},
			{"uv", o.UV},
			{"solar_wm2", o.SolarWm2},
			{"rain_in", conv(o.RainMm, model.MmToInch)},
			{"strike_dist_mi", conv(o.StrikeDistKm, model.KmToMile)},
			{"strike_count", o.StrikeCount},
			{"battery_v", o.BatteryV},
		}
	}
	return []exportField{
		{"wind_lull_mps", conv(o.WindLullMps, id)},
		{"wind_avg_mps", o.WindAvgMps},
		{"wind_gust_mps", o.WindGustMps},
		{"wind_dir_deg", o.WindDirDeg},
		{"pressure_mb", o.PressureMb},
		{"air_temp_c", o.AirTempC},
		{"humidity_pct", o.HumidityPct},
		{"illuminance_lux", o.IlluminanceLux},
		{"uv", o.UV},
		{"solar_wm2", o.SolarWm2},
		{"rain_mm", o.RainMm},
		{"strike_dist_km", o.StrikeDistKm},
		{"strike_count", o.StrikeCount},
		{"battery_v", o.BatteryV},
	}
}

// fmtNum preserves SI precision. It rounds converted values to four decimals.
func fmtNum(v float64, us bool) string {
	if us {
		v = math.Round(v*1e4) / 1e4
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func cmdExport(args []string) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	describe(fs, "tempest export: stream a date range of observations to stdout as CSV or\nJSON Lines, in SI or US units.",
		"tempest export > archive.csv",
		"tempest export --start 2024-06-01 --end 2024-06-30 --units us > june.csv",
		"tempest export --format jsonl | jq .temp_c")
	db := fs.String("db", "", "path to the tempest.sqlite archive (or env TEMPEST_DB)")
	format := fs.String("format", "csv", "output format: csv or jsonl")
	units := fs.String("units", "si", "unit system: si (stored values) or us (display units)")
	start := fs.String("start", "", "start date YYYY-MM-DD in local time (default: whole archive)")
	end := fs.String("end", "", "end date YYYY-MM-DD in local time, inclusive (default: today)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	*format = strings.ToLower(*format)
	if *format != "csv" && *format != "jsonl" {
		return usagef("--format must be csv or jsonl")
	}
	*units = strings.ToLower(*units)
	if *units != "si" && *units != "us" {
		return usagef("--units must be si or us")
	}
	us := *units == "us"

	startEpoch, endEpoch, err := exportRange(*start, *end)
	if err != nil {
		return usageErr{err}
	}

	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}
	dbPath, err := config.ResolveDB(ctx, *db)
	if err != nil {
		return err
	}
	if dbPath == "" {
		return fmt.Errorf("no archive configured: set --db/TEMPEST_DB, or run `tempest setup`")
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Close()) }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { err = errors.Join(err, out.Flush()) }()

	if *format == "csv" {
		return exportCSV(ctx, st, out, startEpoch, endEpoch, us)
	}
	return exportJSONL(ctx, st, out, startEpoch, endEpoch, us)
}

// exportRange resolves optional local start/end dates to an inclusive epoch
// range; an empty start means the whole archive, an empty end means today.
func exportRange(start, end string) (int64, int64, error) {
	var startEpoch int64
	if start != "" {
		t, err := time.ParseInLocation("2006-01-02", start, time.Local)
		if err != nil {
			return 0, 0, fmt.Errorf("--start must be YYYY-MM-DD")
		}
		startEpoch = t.Unix()
	}
	endEpoch := time.Now().Unix()
	if end != "" {
		t, err := time.ParseInLocation("2006-01-02", end, time.Local)
		if err != nil {
			return 0, 0, fmt.Errorf("--end must be YYYY-MM-DD")
		}
		endEpoch = t.AddDate(0, 0, 1).Unix() - 1 // include the whole end day
	}
	if startEpoch > endEpoch {
		return 0, 0, fmt.Errorf("--start must not be after --end")
	}
	return startEpoch, endEpoch, nil
}

func exportCSV(ctx context.Context, st *store.Store, out *bufio.Writer, start, end int64, us bool) error {
	w := csv.NewWriter(out)
	// Header: epoch + local time, then the sensor columns in field order.
	header := []string{"epoch", "time"}
	for _, f := range obsFields(model.Obs{}, us) {
		header = append(header, f.Name)
	}
	if err := w.Write(header); err != nil {
		return err
	}
	err := st.EachObs(ctx, start, end, func(o model.Obs) error {
		rec := make([]string, 0, len(header))
		rec = append(rec, strconv.FormatInt(o.Epoch, 10),
			time.Unix(o.Epoch, 0).Local().Format(time.RFC3339))
		for _, f := range obsFields(o, us) {
			if f.Val == nil {
				rec = append(rec, "")
			} else {
				rec = append(rec, fmtNum(*f.Val, us))
			}
		}
		return w.Write(rec)
	})
	if err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func exportJSONL(ctx context.Context, st *store.Store, out *bufio.Writer, start, end int64, us bool) error {
	return st.EachObs(ctx, start, end, func(o model.Obs) error {
		var b strings.Builder
		b.WriteByte('{')
		fmt.Fprintf(&b, `"epoch":%d,"time":%q`, o.Epoch,
			time.Unix(o.Epoch, 0).Local().Format(time.RFC3339))
		for _, f := range obsFields(o, us) {
			if f.Val != nil {
				b.WriteString(`,"`)
				b.WriteString(f.Name)
				b.WriteString(`":`)
				b.WriteString(fmtNum(*f.Val, us))
			}
		}
		b.WriteString("}\n")
		_, err := out.WriteString(b.String())
		return err
	})
}
