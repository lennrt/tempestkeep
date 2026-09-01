// Package mcpapp serves Tempest weather data over MCP stdio.
// The server opens the archive read-only unless archive writes are configured.
// Stdout contains MCP messages only. Logs use stderr.
package mcpapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lennrt/tempestkeep/internal/version"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options contains resolved MCP inputs. Run borrows these values for the call.
type Options struct {
	Token    string
	DBPath   string
	ReadOnly bool
}

// Run serves MCP until the context is canceled or the transport fails.
func Run(ctx context.Context, opts Options) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	var (
		live   *liveSource
		st     *store.Store
		writer *store.Writer
	)

	// Resolve the station on first use. An unavailable API must not delay startup.
	if opts.Token != "" {
		client, err := newAPIClient(opts.Token)
		if err != nil {
			return err
		}
		live = &liveSource{client: client}
	}

	// Open the writer first. It creates the schema before the read handle opens.
	if opts.Token != "" && opts.DBPath != "" && !opts.ReadOnly {
		writer, err = store.OpenWriter(ctx, opts.DBPath)
		if err != nil {
			return fmt.Errorf("open configured archive for writes: %w", err)
		}
		defer func() { err = errors.Join(err, writer.Close()) }()
		log.Print("archive write access is ready")
	}

	if opts.DBPath != "" {
		st, err = store.Open(ctx, opts.DBPath)
		if err != nil {
			return fmt.Errorf("open configured archive for reads: %w", err)
		}
		defer func() { err = errors.Join(err, st.Close()) }()
		if writer == nil {
			log.Printf("archive read access is ready%s", readOnlyNote(opts.ReadOnly))
		}
	}

	if live == nil && st == nil {
		return fmt.Errorf("no data source: set TEMPEST_TOKEN and/or --db/TEMPEST_DB")
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "tempestkeep", Version: version.String()}, nil)
	registerTools(srv, live, st)
	if writer != nil && live != nil {
		registerArchiveTools(srv, live, writer)
	}

	log.Printf("tempestkeep mcp %s ready (live=%v, archive=%v, writable=%v)", version.String(), live != nil, st != nil, writer != nil)
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
