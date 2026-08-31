package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/collect"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// errNoToken is shared by commands that require the live API.
var errNoToken = errors.New("no token; set TEMPEST_TOKEN in a private .env file or process environment")

const (
	maxBackupKeep    = 365
	maxBackupEntries = 10_000
)

// cmdCollect updates or creates the local archive.
func cmdCollect(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fs := flag.NewFlagSet("collect", flag.ContinueOnError)
	describe(fs, "Build or refresh the local SQLite archive from the WeatherFlow REST API.\nA repeated run resumes after the last stored observation. Ctrl-C cancels the run.",
		"tempest collect",
		"tempest collect --backfill-start 2023-01-01",
		"tempest collect --quiet   # for cron; progress off, errors still print")
	db := fs.String("db", "", "archive path (or env TEMPEST_DB; default ./tempest.sqlite)")
	deviceID := fs.Int("device-id", 0, "Tempest device id (or env TEMPEST_DEVICE_ID; auto-discovered if unset)")
	backfillStart := fs.String("backfill-start", "", "earliest date to backfill on a fresh archive, YYYY-MM-DD (default: walk back until history ends)")
	backupKeep := fs.Int("backup-keep", 7, "backup snapshots to keep in ./backups (0 disables)")
	noBackup := fs.Bool("no-backup", false, "skip the post-run backup snapshot")
	throttleMs := fs.Int("throttle-ms", 400, "pause between REST requests, milliseconds")
	quiet := fs.Bool("quiet", false, "suppress progress/status output (errors still print)")
	fs.BoolVar(quiet, "q", false, "shorthand for --quiet")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}

	// Env fallbacks for the archive-shaping knobs, so a cron job can carry its
	// whole config in .env. An explicit flag wins; only an unset flag defers.
	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
	if *backfillStart == "" {
		*backfillStart = os.Getenv("TEMPEST_BACKFILL_START")
	}
	if !setFlags["backup-keep"] {
		if e := os.Getenv("TEMPEST_BACKUP_KEEP"); e != "" {
			n, perr := strconv.Atoi(e)
			if perr != nil {
				return fmt.Errorf("TEMPEST_BACKUP_KEEP must be an integer")
			}
			*backupKeep = n
		}
	}
	if !setFlags["throttle-ms"] {
		if e := os.Getenv("TEMPEST_THROTTLE_MS"); e != "" {
			n, perr := strconv.Atoi(e)
			if perr != nil {
				return fmt.Errorf("TEMPEST_THROTTLE_MS must be an integer")
			}
			*throttleMs = n
		}
	}
	if *deviceID < 0 {
		return usagef("--device-id must be positive")
	}
	if *backupKeep < 0 || *backupKeep > maxBackupKeep {
		return usagef("--backup-keep must be between 0 and %d", maxBackupKeep)
	}
	if *throttleMs < 0 || *throttleMs > 60_000 {
		return usagef("--throttle-ms must be between 0 and 60000")
	}

	// Status/progress is diagnostics, not data: it goes to stderr (so a piped
	// stdout stays clean) and is silenced by --quiet. Errors always print.
	logf := func(format string, a ...any) {
		if !*quiet {
			fmt.Fprintf(os.Stderr, format, a...)
		}
	}

	tok := os.Getenv("TEMPEST_TOKEN")
	if tok == "" {
		return errNoToken
	}
	dbPath := config.FirstNonEmpty(*db, os.Getenv("TEMPEST_DB"), config.DefaultDB)

	client, err := newAPIClient(tok)
	if err != nil {
		return err
	}

	dev := *deviceID
	if dev == 0 {
		if e := os.Getenv("TEMPEST_DEVICE_ID"); e != "" {
			// A malformed value must not silently fall through to discovery,
			// which could pick a different device and split the archive.
			n, perr := strconv.Atoi(e)
			if perr != nil {
				return fmt.Errorf("TEMPEST_DEVICE_ID must be an integer")
			}
			dev = n
		}
	}
	if dev < 0 {
		return fmt.Errorf("TEMPEST_DEVICE_ID must be positive")
	}
	if dev == 0 {
		_, discovered, err := client.FindTempestDevice(ctx)
		if err != nil {
			return err
		}
		dev = discovered
		logf("Using the discovered Tempest device (override with --device-id).\n")
	}

	var startEpoch int64
	if *backfillStart != "" {
		// Local time, like every other date the tools accept ("2023-06-01" means
		// the user's June 1st, not UTC's).
		t, err := time.ParseInLocation("2006-01-02", *backfillStart, time.Local)
		if err != nil {
			return usagef("--backfill-start must be YYYY-MM-DD")
		}
		if t.After(time.Now()) {
			// A future start would make an empty range whose walk silently
			// fetches nothing; reject it instead of reporting a successful no-op.
			return usagef("--backfill-start must not be in the future")
		}
		startEpoch = t.Unix()
	}

	w, err := store.OpenWriter(ctx, dbPath)
	if err != nil {
		return err
	}

	if before, err := w.Coverage(ctx, dev); err != nil {
		fmt.Fprintln(os.Stderr, "warning: archive coverage is unavailable")
	} else {
		logf("Archive before collection: %d rows%s\n", before.Count, span(before))
	}

	throttle := time.Duration(*throttleMs) * time.Millisecond
	bf, err := collect.New(client, w, dev,
		collect.WithThrottle(throttle),
		collect.WithProgress(func(p collect.Progress) error {
			logf("  through %s: %d fetched, %d new rows\n",
				time.Unix(p.Through, 0).Format("2006-01-02"), p.Fetched, p.RowsAdded)
			return nil
		}),
	)
	if err != nil {
		return errors.Join(err, w.Close())
	}
	res, collectErr := bf.Collect(ctx, time.Now().Unix(), startEpoch)

	// Restore the default interrupt behavior before cleanup and backup.
	stop()

	// Cleanup runs on a fresh context: after Ctrl-C the walk context is dead,
	// and the checkpoint/coverage reads below must still go through.
	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCleanup()

	after, coverageErr := w.Coverage(cleanupCtx, dev)
	if coverageErr == nil {
		logf("Mode %s: fetched %d, added %d new. Total %d rows%s\n",
			res.Mode, res.Fetched, res.RowsAdded, after.Count, span(after))
	}

	// Flush WAL so the snapshot copy below captures every committed row.
	checkpointErr := w.Checkpoint(cleanupCtx)
	closeErr := w.Close()

	var operationErr error
	switch {
	case errors.Is(collectErr, context.Canceled):
		operationErr = errors.New("interrupted; progress is saved, re-run 'tempest collect' to resume")
	case collectErr != nil:
		operationErr = fmt.Errorf("collection stopped early (re-run to resume): %w", collectErr)
	}
	cleanupErr := errors.Join(
		wrapIfError("archive coverage check failed", coverageErr),
		wrapIfError("archive checkpoint failed; committed data remains in the archive", checkpointErr),
		closeErr,
	)
	if err := errors.Join(operationErr, cleanupErr); err != nil {
		return err
	}

	if !*noBackup {
		if dest, err := snapshot(dbPath, *backupKeep); err != nil {
			return errors.New("backup failed; the archive is intact")
		} else if dest != "" {
			logf("Backup created.\n")
		}
	}
	return nil
}

func wrapIfError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// cmdListDevices implements `tempest list-devices`: print the stations and
// devices the token can see, so a user can pick a --device-id.
func cmdListDevices(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fs := flag.NewFlagSet("list-devices", flag.ContinueOnError)
	describe(fs, "tempest list-devices: list the stations and devices your token can see,\nso you can pick a --device-id for collect.",
		"tempest list-devices",
		"tempest list-devices --format json | jq '.[].devices'")
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

	tok := os.Getenv("TEMPEST_TOKEN")
	if tok == "" {
		return errNoToken
	}
	client, err := newAPIClient(tok)
	if err != nil {
		return err
	}
	stations, err := client.Stations(ctx)
	if err != nil {
		return err
	}

	if *format == "json" {
		return writeDevicesJSON(os.Stdout, stations)
	}

	if len(stations) == 0 {
		fmt.Println("No stations found for this token.")
		return nil
	}
	for _, s := range stations {
		fmt.Printf("Station %d  %q  (%s)\n", s.StationID, s.Name, s.Timezone)
		if len(s.Devices) == 0 {
			fmt.Println("  (no devices)")
		}
		for _, d := range s.Devices {
			fmt.Printf("  device %d  type %s  serial %s\n", d.DeviceID, d.DeviceType, d.SerialNumber)
		}
	}
	return nil
}

// deviceJSON / stationJSON are the machine-readable shape of `list-devices
// --format json`: a stable, scriptable listing so picking a --device-id doesn't
// require screen-scraping the text output.
type deviceJSON struct {
	DeviceID int    `json:"device_id"`
	Type     string `json:"type"`
	Serial   string `json:"serial"`
}

type stationJSON struct {
	StationID int          `json:"station_id"`
	Name      string       `json:"name"`
	Timezone  string       `json:"timezone"`
	Devices   []deviceJSON `json:"devices"`
}

func writeDevicesJSON(w io.Writer, stations []api.Station) error {
	out := make([]stationJSON, 0, len(stations))
	for _, s := range stations {
		sj := stationJSON{StationID: s.StationID, Name: s.Name, Timezone: s.Timezone, Devices: []deviceJSON{}}
		for _, d := range s.Devices {
			sj.Devices = append(sj.Devices, deviceJSON{DeviceID: d.DeviceID, Type: d.DeviceType, Serial: d.SerialNumber})
		}
		out = append(out, sj)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// span renders a coverage range as " (2023-01-01 -> 2024-06-30)" for logging, or
// "" when the archive is empty.
func span(c store.Coverage) string {
	if !c.MinEpoch.Valid || !c.MaxEpoch.Valid {
		return ""
	}
	lo := time.Unix(c.MinEpoch.Int64, 0).Format("2006-01-02")
	hi := time.Unix(c.MaxEpoch.Int64, 0).Format("2006-01-02")
	return fmt.Sprintf(" (%s -> %s)", lo, hi)
}

// snapshot copies the archive into ./backups with a UTC timestamp and prunes all
// but the newest keep snapshots. keep <= 0 disables snapshots.
func snapshot(dbPath string, keep int) (string, error) {
	if keep <= 0 {
		return "", nil
	}
	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("backup path must be a directory, not a symlink")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", err
	}
	stem := stem(filepath.Base(dbPath))
	stamp := time.Now().UTC().Format("20060102-150405")
	dest := filepath.Join(dir, fmt.Sprintf("%s-%s.sqlite", stem, stamp))
	if err := copyFile(dbPath, dest); err != nil {
		return "", err
	}
	return dest, pruneBackups(dir, stem, keep)
}

func pruneBackups(dir, stem string, keep int) (err error) {
	if keep < 1 || keep > maxBackupKeep {
		return fmt.Errorf("backup retention must be in 1..%d", maxBackupKeep)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	entries, readErr := directory.ReadDir(maxBackupEntries + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if len(entries) > maxBackupEntries {
		return fmt.Errorf("backup directory exceeds %d entries", maxBackupEntries)
	}
	prefix := stem + "-"
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".sqlite") {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".sqlite")
		if _, err := time.Parse("20060102-150405", stamp); err == nil {
			names = append(names, name)
		}
	}
	if len(names) <= keep {
		return nil
	}
	sort.Strings(names)
	var removeErrors []error
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErrors = append(removeErrors, err)
		}
	}
	return errors.Join(removeErrors...)
}

// copyFile writes to a temp name and links it into place, so a failed copy never
// leaves a truncated snapshot that rotation would keep while pruning good ones.
// The .tmp suffix also keeps partials out of pruneBackups's *.sqlite glob.
func copyFile(src, dst string) (err error) {
	sourceInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		return errors.New("backup source must be a regular file, not a symlink")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	openedInfo, err := in.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(sourceInfo, openedInfo) {
		return errors.New("backup source changed while it was opened")
	}
	out, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer func() {
		if removeErr := os.Remove(tmp); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, out.Close())
		}
	}()
	if err := out.Chmod(0o600); err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	closed = true
	// A hard link publishes the complete file atomically and fails if the
	// destination already exists. It never overwrites a prior backup.
	if err := os.Link(tmp, dst); err != nil {
		return err
	}
	return nil
}

func stem(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}
