package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// MaxArchiveSourceBytes bounds stored source text before it is loaded into Go.
const MaxArchiveSourceBytes = 1024

// ArchiveObs is an observation with its stored provenance and report intervals.
// Sensor values retain SI units. Nil metadata means a stored NULL or a column
// absent from an older archive. Source is stored text, not inferred provenance.
type ArchiveObs struct {
	model.Obs
	DeviceID            *int64   `json:"device_id,omitempty"`
	ReportIntervalMin   *float64 `json:"report_interval_min,omitempty"`
	WindIntervalSeconds *float64 `json:"wind_interval_s,omitempty"`
	PrecipType          *float64 `json:"precip_type,omitempty"`
	Source              *string  `json:"source,omitempty"`
}

type archiveProjection struct {
	Columns, DeviceID, ReportIntervalMin, WindIntervalSeconds, PrecipType, Source, SourceBytes string
}

func missingArchiveProjection() archiveProjection {
	return archiveProjection{
		Columns: obsColumns, DeviceID: "NULL", ReportIntervalMin: "NULL",
		WindIntervalSeconds: "NULL", PrecipType: "NULL", Source: "NULL", SourceBytes: "NULL",
	}
}

func archiveColumns(ctx context.Context, db *sql.DB) (_ archiveProjection, err error) {
	projection := missingArchiveProjection()
	rows, err := db.QueryContext(ctx, qryArchiveMetadata)
	if err != nil {
		return projection, archiveFailure("read optional archive columns", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close optional archive columns", rows.Close())) }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return projection, archiveFailure("scan optional archive column", err)
		}
		// Do not interpolate database-owned identifiers. Even the known names
		// are replaced by these static expressions before rendering the query.
		switch name {
		case "device_id":
			projection.DeviceID = "device_id"
		case "report_interval_min":
			projection.ReportIntervalMin = "report_interval_min"
		case "wind_interval":
			projection.WindIntervalSeconds = "wind_interval"
		case "precip_type":
			projection.PrecipType = "precip_type"
		case "source":
			projection.Source = fmt.Sprintf("CASE WHEN length(CAST(source AS BLOB)) <= %d THEN source ELSE NULL END", MaxArchiveSourceBytes)
			projection.SourceBytes = "length(CAST(source AS BLOB))"
		}
	}
	return projection, archiveFailure("read optional archive columns", rows.Err())
}

// EachArchiveObs streams observations and stored metadata in the inclusive
// epoch range, oldest first. It performs read-only schema and observation queries
// without buffering the range. Missing optional columns yield nil fields.
// Source must be valid UTF-8 and at most MaxArchiveSourceBytes bytes; an invalid
// source returns ErrInvalidArchive without truncation. A callback error stops
// iteration and remains classifiable with errors.Is. Cancellation stops reads.
func (s *Store) EachArchiveObs(ctx context.Context, startEpoch, endEpoch int64, fn func(ArchiveObs) error) (err error) {
	if err := validateEpochRange(startEpoch, endEpoch); err != nil {
		return err
	}
	if fn == nil {
		return fmt.Errorf("%w: archive observation callback is nil", ErrInvalidArgument)
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	projection, err := archiveColumns(ctx, db)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, execTemplate(archiveRangeTmpl, projection), startEpoch, endEpoch)
	if err != nil {
		return archiveFailure("read archive observations", err)
	}
	defer func() { err = errors.Join(err, archiveFailure("close archive observations", rows.Close())) }()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var device, sourceBytes sql.NullInt64
		var report, wind, precip sql.NullFloat64
		var source sql.NullString
		o, err := scanObs(rows, &device, &report, &wind, &precip, &source, &sourceBytes)
		if err != nil {
			return archiveFailure("scan archive observation", err)
		}
		if sourceBytes.Valid && sourceBytes.Int64 > MaxArchiveSourceBytes {
			return fmt.Errorf("%w: observation source exceeds %d bytes", ErrInvalidArchive, MaxArchiveSourceBytes)
		}
		if source.Valid && !utf8.ValidString(source.String) {
			return fmt.Errorf("%w: observation source is not valid UTF-8", ErrInvalidArchive)
		}
		for _, value := range []sql.NullFloat64{report, wind, precip} {
			if value.Valid && (math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0)) {
				return fmt.Errorf("%w: observation metadata contains a non-finite value", ErrInvalidArchive)
			}
		}
		archive := ArchiveObs{Obs: o, ReportIntervalMin: nf(report), WindIntervalSeconds: nf(wind), PrecipType: nf(precip)}
		if device.Valid {
			archive.DeviceID = new(device.Int64)
		}
		if source.Valid {
			archive.Source = new(source.String)
		}
		if err := fn(archive); err != nil {
			return err
		}
	}
	return archiveFailure("read archive observations", rows.Err())
}
