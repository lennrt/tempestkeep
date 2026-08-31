// Command tempest-mcp serves Tempest weather data over MCP stdio.
// TEMPEST_TOKEN enables live tools. --db or TEMPEST_DB enables archive tools.
// The server opens the archive read-only unless archive writes are configured.
// Stdout contains MCP messages only. Logs use stderr.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/lennrt/tempestkeep/internal/version"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run returns errors so deferred cleanup runs before log.Fatal exits.
func run() (err error) {
	dbFlag := flag.String("db", "", "path to the tempest.sqlite archive (or env TEMPEST_DB)")
	readOnlyFlag := flag.Bool("read-only", false, "never write to the archive: disable the backfill/sync tools (or env TEMPEST_READ_ONLY)")
	versionFlag := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("tempest-mcp %s\n", version.String())
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}

	token := os.Getenv("TEMPEST_TOKEN")
	dbPath, err := config.ResolveDB(ctx, *dbFlag)
	if err != nil {
		return err
	}
	envReadOnly, err := readOnlyEnv()
	if err != nil {
		return err
	}
	readOnly := *readOnlyFlag || envReadOnly

	var (
		live   *liveSource
		st     *store.Store
		writer *store.Writer
	)

	// Resolve the station on first use. An unavailable API must not delay startup.
	if token != "" {
		client, err := newAPIClient(token)
		if err != nil {
			return err
		}
		live = &liveSource{client: client}
	}

	// Open the writer first. It creates the schema before the read handle opens.
	if token != "" && dbPath != "" && !readOnly {
		writer, err = store.OpenWriter(ctx, dbPath)
		if err != nil {
			return fmt.Errorf("open configured archive for writes: %w", err)
		}
		defer func() { err = errors.Join(err, writer.Close()) }()
		log.Print("archive write access is ready")
	}

	if dbPath != "" {
		st, err = store.Open(ctx, dbPath)
		if err != nil {
			return fmt.Errorf("open configured archive for reads: %w", err)
		}
		defer func() { err = errors.Join(err, st.Close()) }()
		if writer == nil {
			log.Printf("archive read access is ready%s", readOnlyNote(readOnly))
		}
	}

	if live == nil && st == nil {
		return fmt.Errorf("no data source: set TEMPEST_TOKEN and/or --db/TEMPEST_DB")
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "tempest-mcp", Version: version.String()}, nil)
	registerTools(srv, live, st)
	if writer != nil && live != nil {
		registerArchiveTools(srv, live, writer)
	}

	log.Printf("tempest-mcp %s ready (live=%v, archive=%v, writable=%v)", version.String(), live != nil, st != nil, writer != nil)
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("MCP server stopped with an error")
	}
	return nil
}

// warnTimezoneMismatch reports a zone mismatch. Calendar queries use the
// process zone, so the operator must set TZ to the station zone.
func warnTimezoneMismatch(stationTZ string) {
	if stationTZ == "" {
		return
	}
	loc, err := time.LoadLocation(stationTZ)
	if err != nil {
		return
	}
	now := time.Now()
	if timezoneOffsetsDiffer(time.Local, loc, now) {
		log.Print("warning: process timezone differs from the station timezone; set TZ to the station timezone before using calendar summaries")
	}
}

// timezoneOffsetsDiffer checks two seasons to detect different daylight rules.
func timezoneOffsetsDiffer(process, station *time.Location, now time.Time) bool {
	for _, at := range []time.Time{now, now.AddDate(0, 6, 0)} {
		_, processOffset := at.In(process).Zone()
		_, stationOffset := at.In(station).Zone()
		if processOffset != stationOffset {
			return true
		}
	}
	return false
}

// readOnlyNote marks an archive that was configured for read-only access.
func readOnlyNote(readOnly bool) string {
	if readOnly {
		return " (read-only)"
	}
	return ""
}

// readOnlyEnv parses TEMPEST_READ_ONLY. It rejects unknown values to prevent an
// invalid read-only setting from enabling write tools.
func readOnlyEnv() (bool, error) {
	const key = "TEMPEST_READ_ONLY"
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true, nil
	case "", "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("%s must be a boolean (1/0, true/false, yes/no, on/off)", key)
}

// liveSource resolves and caches one station and device. Failed lookups are not
// cached, so later calls can retry.
type liveSource struct {
	client   *api.Client
	mu       sync.Mutex
	station  *api.Station
	deviceID int
}

// resolveStation returns the cached station or resolves it. It is safe for
// concurrent use. It caches only successful results.
func (l *liveSource) resolveStation(ctx context.Context) (*api.Station, error) {
	l.mu.Lock()
	if l.station != nil {
		station := l.station
		l.mu.Unlock()
		return station, nil
	}
	l.mu.Unlock()

	stations, err := l.client.Stations(ctx)
	if err != nil {
		return nil, err
	}
	s, deviceID, ok := api.PickTempestDevice(stations)
	if !ok {
		return nil, fmt.Errorf("no stations found for this token")
	}
	l.mu.Lock()
	if l.station == nil {
		l.station, l.deviceID = s, deviceID
	}
	station := l.station
	l.mu.Unlock()
	log.Print("live station access is ready")
	warnTimezoneMismatch(s.Timezone)
	return station, nil
}

// resolveDevice returns the device id whose history the archive tools collect,
// resolving the station first if needed.
func (l *liveSource) resolveDevice(ctx context.Context) (int, error) {
	if _, err := l.resolveStation(ctx); err != nil {
		return 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.deviceID, nil
}
