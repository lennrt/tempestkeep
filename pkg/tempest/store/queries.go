package store

// The SQL behind Store and Writer lives in sql/*.sql (one file per statement,
// embedded at compile time) so each query can carry its own commentary and be
// reviewed as SQL rather than as a Go string literal. Most statements are
// static text; the two that need light templating (latest.sql shares the obs
// column list, epoch_of.sql substitutes an internal column name) go through
// text/template, rendered once where possible.

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed sql/*.sql
var sqlFS embed.FS

// loadedSQL records every file mustQuery has read, so the test suite can prove
// that no embedded .sql file is left unloaded.
var loadedSQL = map[string]bool{}

// mustQuery returns the contents of sql/<name>. The files are compile-time
// assets, so a missing or unreadable name is a programmer error worth a panic
// at package init.
func mustQuery(name string) string {
	b, err := sqlFS.ReadFile("sql/" + name)
	if err != nil {
		panic(fmt.Sprintf("store: embedded query %s: %v", name, err))
	}
	loadedSQL[name] = true
	return string(b)
}

// mustTemplate parses sql/<name> as a text/template, panicking on error for
// the same reason mustQuery does.
func mustTemplate(name string) *template.Template {
	return template.Must(template.New(name).Parse(mustQuery(name)))
}

// execTemplate renders a query template; the templates and their data are
// compile-time constants, so a render error is a panic.
func execTemplate(t *template.Template, data any) string {
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		panic(fmt.Sprintf("store: render %s: %v", t.Name(), err))
	}
	return sb.String()
}

// Static statements, loaded once at package init. See the .sql files for the
// commentary on each query.
var (
	qryHasObsTable     = mustQuery("has_obs_table.sql")
	qryCoverage        = mustQuery("coverage.sql")
	qryGaps            = mustQuery("gaps.sql")
	qryDayRollup       = mustQuery("day_rollup.sql")
	qryHourRollup      = mustQuery("hour_rollup.sql")
	qryLightningRollup = mustQuery("lightning_rollup.sql")
	qrySolarRollup     = mustQuery("solar_rollup.sql")
	qryWindRollup      = mustQuery("wind_rollup.sql")
	qryComfortRollup   = mustQuery("comfort_rollup.sql")
	qryPressureRollup  = mustQuery("pressure_rollup.sql")
	qrySensorHealth    = mustQuery("sensor_health.sql")
	qryPressureAt      = mustQuery("pressure_at.sql")
	qryRecordsExtremes = mustQuery("records_extremes.sql")
	qryThisDay         = mustQuery("this_day.sql")
	qryWindRoseSectors = mustQuery("wind_rose_sectors.sql")
	qryWindRoseCalm    = mustQuery("wind_rose_calm.sql")
	qrySeries          = mustQuery("series.sql")
	qrySchema          = mustQuery("schema.sql")
	qryInsertObs       = mustQuery("insert_obs.sql")
	qryWatermark       = mustQuery("watermark.sql")
	qryWriterCoverage  = mustQuery("writer_coverage.sql")
	qryMetaGet         = mustQuery("meta_get.sql")
	qryMetaSet         = mustQuery("meta_set.sql")
	qryMetaBindDevice  = mustQuery("meta_bind_device.sql")
	qryDeviceIDs       = mustQuery("device_ids.sql")
)

// qryLatest and qryRange share obsColumns into their templates, so the SELECT
// list always matches the order scanObs expects. Rendered once at init.
var (
	qryLatest = execTemplate(mustTemplate("latest.sql"), struct{ Columns string }{obsColumns})
	qryRange  = execTemplate(mustTemplate("range.sql"), struct{ Columns string }{obsColumns})
)

// epochOfTmpl substitutes an internal column name into epoch_of.sql. The
// column is always one of our own constants, never user input.
var epochOfTmpl = mustTemplate("epoch_of.sql")

// epochOfSQL renders the epoch-lookup query for one obs_st column.
func epochOfSQL(col string) string {
	return execTemplate(epochOfTmpl, struct{ Column string }{col})
}
