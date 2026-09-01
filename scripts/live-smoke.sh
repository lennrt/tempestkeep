#!/bin/sh
# Run a bounded live check against the WeatherFlow service.
set -eu

if [ -z "${TEMPEST_TOKEN:-}" ]; then
	echo "TEMPEST_TOKEN is required for the live smoke test." >&2
	exit 2
fi

smoke_dir=$(mktemp -d /tmp/tempestkeep-live.XXXXXX)
archive_path="$smoke_dir/archive.sqlite"
cleanup() {
	rm -f -- "$archive_path" "$archive_path-wal" "$archive_path-shm" "$archive_path-journal"
	rmdir -- "$smoke_dir"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

umask 077
export TEMPEST_API_BASE="https://swd.weatherflow.com/swd/rest"

make check-go build
./bin/tempestkeep list-devices --format json >/dev/null
./bin/tempestkeep now --format json >/dev/null
./bin/tempestkeep now --forecast --hours 1 --days 1 --format json >/dev/null

start_date=$(date -u +%F)
./bin/tempestkeep collect \
	--db "$archive_path" \
	--backfill-start "$start_date" \
	--throttle-ms 0 \
	--no-backup \
	--quiet

TEMPEST_TOKEN= ./bin/tempestkeep now --db "$archive_path" --format json >/dev/null
