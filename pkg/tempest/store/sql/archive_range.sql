-- Stream the existing sensor fields plus optional stored metadata. Every
-- template expression is an internal constant selected from a static allowlist.
-- Source is selected only when its byte length is within the supported bound;
-- its length is returned separately so an oversized value fails without being
-- loaded into Go memory. Missing legacy columns use NULL expressions.
-- Params: startEpoch, endEpoch.
SELECT {{.Columns}}, {{.DeviceID}}, {{.ReportIntervalMin}}, {{.WindIntervalSeconds}},
       {{.PrecipType}}, {{.Source}}, {{.SourceBytes}}
FROM obs_st
WHERE epoch BETWEEN ? AND ?
ORDER BY epoch
