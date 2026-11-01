package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

const statusCoverageNote = "Stored observations and internal gaps do not establish continuous coverage or sensor availability."

type archiveStatusReport struct {
	Observations      int64       `json:"observations"`
	FirstObs          *string     `json:"first_obs"`
	LastObs           *string     `json:"last_obs"`
	LastObsAgeSeconds *int64      `json:"last_obs_age_seconds"`
	Fresh             bool        `json:"fresh"`
	BackfillComplete  bool        `json:"backfill_complete"`
	GapThreshold      int64       `json:"gap_threshold_seconds"`
	GapLimit          int         `json:"gap_limit"`
	Gaps              []store.Gap `json:"gaps"`
	Note              string      `json:"note"`
}

func cmdStatus(args []string) (err error) {
	ctx, stop := signalContext()
	defer stop()
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	describe(fs, "tempestkeep status: inspect archive coverage, freshness, backfill state, and\nthe ten largest internal gaps over one hour. Read-only; no token or network needed.",
		"tempestkeep status --db ./tempest.sqlite",
		"tempestkeep status --format json | jq .gaps")
	db := fs.String("db", "", "path to the tempest.sqlite archive (or env TEMPEST_DB)")
	format := fs.String("format", "text", "output format: text or json")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	*format = strings.ToLower(*format)
	if *format != "text" && *format != "json" {
		return usagef("--format must be text or json")
	}
	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}
	path, err := config.ResolveDB(ctx, *db)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Close()) }()
	report, err := readArchiveStatus(ctx, st, time.Now())
	if err != nil {
		return err
	}
	out := bufio.NewWriter(os.Stdout)
	defer func() { err = errors.Join(err, out.Flush()) }()
	return writeArchiveStatus(out, report, *format)
}

func readArchiveStatus(ctx context.Context, st *store.Store, now time.Time) (archiveStatusReport, error) {
	r := archiveStatusReport{GapThreshold: 3600, GapLimit: 10, Gaps: []store.Gap{}, Note: statusCoverageNote}
	cov, err := st.Coverage(ctx)
	if err != nil {
		return r, err
	}
	r.Observations = cov.Count
	if cov.MinEpoch.Valid && cov.MaxEpoch.Valid {
		r.FirstObs = new(time.Unix(cov.MinEpoch.Int64, 0).Local().Format(time.RFC3339))
		r.LastObs = new(time.Unix(cov.MaxEpoch.Int64, 0).Local().Format(time.RFC3339))
		r.LastObsAgeSeconds = new(now.Unix() - cov.MaxEpoch.Int64)
		r.Fresh = *r.LastObsAgeSeconds >= 0 && *r.LastObsAgeSeconds < r.GapThreshold
	}
	gaps, err := st.Gaps(ctx, r.GapThreshold, r.GapLimit)
	if err != nil {
		return r, err
	}
	r.Gaps = append(r.Gaps, gaps...)
	marker, exists, err := st.Meta(ctx, collect.MetaBackfillComplete)
	if err != nil {
		return r, err
	}
	if exists && marker != "0" && marker != "1" {
		return r, fmt.Errorf("%w: archive backfill completion state is invalid", collect.ErrInvalidCheckpoint)
	}
	r.BackfillComplete = marker == "1"
	return r, nil
}

func writeArchiveStatus(w io.Writer, r archiveStatusReport, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	out := textOutput{writer: w}
	out.printf("Archive status\nObservations: %d\n", r.Observations)
	if r.LastObs == nil {
		out.println("Archive is empty; no observation timestamps are available.")
	} else {
		out.printf("First: %s\nLast: %s\nNewest age: %d seconds\n", *r.FirstObs, *r.LastObs, *r.LastObsAgeSeconds)
		out.printf("Fresh (age 0..3599 seconds): %s\n", yesNo(r.Fresh))
	}
	out.printf("Backfill complete: %s\n", yesNo(r.BackfillComplete))
	out.printf("Largest internal gaps over %d seconds (up to %d):\n", r.GapThreshold, r.GapLimit)
	if len(r.Gaps) == 0 {
		out.println("  No qualifying internal gaps.")
	}
	for _, gap := range r.Gaps {
		out.printf("  %s to %s: %d seconds\n", gap.FromLocal, gap.ToLocal, gap.Seconds)
	}
	out.println(r.Note)
	return out.err
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
