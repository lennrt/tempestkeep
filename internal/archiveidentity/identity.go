// Package archiveidentity verifies optional enrichment between live station
// responses and the single-device archive without changing either data source.
package archiveidentity

import (
	"context"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// MatchesStation reports whether the archive's observed device belongs to the
// station. Store.Open rejects mixed-device archives, so one row establishes its
// identity. Empty archives, legacy schemas without device_id, and read failures
// cannot establish a match; callers should leave optional enrichment absent.
func MatchesStation(ctx context.Context, archive *store.Store, station *api.Station) bool {
	if archive == nil || station == nil || len(station.Devices) == 0 {
		return false
	}
	result, err := archive.Query(ctx, "SELECT device_id FROM obs_st LIMIT 1", 1)
	if err != nil || len(result.Rows) != 1 || len(result.Rows[0]) != 1 {
		return false
	}
	deviceID, ok := result.Rows[0][0].(int64)
	if !ok || deviceID <= 0 {
		return false
	}
	for _, device := range station.Devices {
		if int64(device.DeviceID) == deviceID {
			return true
		}
	}
	return false
}
