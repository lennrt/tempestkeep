#!/bin/sh
# Run the real MCP client/server demo with synthetic data in a private directory.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
umask 077
demo_dir=$(mktemp -d "${TMPDIR:-/tmp}/tempestkeep-mcp-demo.XXXXXX")
demo_pid=
cleanup() {
	if [ -n "$demo_pid" ]; then
		kill "$demo_pid" 2>/dev/null || true
		wait "$demo_pid" 2>/dev/null || true
		demo_pid=
	fi
	rm -f -- "$demo_dir/api-url" "$demo_dir/api.log" \
		"$demo_dir/archive.sqlite" "$demo_dir/archive.sqlite-wal" \
		"$demo_dir/archive.sqlite-shm" "$demo_dir/archive.sqlite-journal"
	rmdir -- "$demo_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

export TZ=UTC
"$repo_root/bin/demoapi" -addr 127.0.0.1:0 -url-file "$demo_dir/api-url" \
	>"$demo_dir/api.log" 2>&1 &
demo_pid=$!

# The readiness file is written only after the local listener is bound.
tries=0
while [ ! -s "$demo_dir/api-url" ]; do
	tries=$((tries + 1))
	if [ "$tries" -ge 200 ] || ! kill -0 "$demo_pid" 2>/dev/null; then
		echo "The synthetic API did not start within ten seconds." >&2
		exit 1
	fi
	sleep 0.05
done

export TEMPEST_API_BASE="$(cat "$demo_dir/api-url")"
export TEMPEST_TOKEN=synthetic-demo-token
export TEMPEST_DB="$demo_dir/archive.sqlite"
export TEMPEST_READ_ONLY=false TEMPEST_CACHE_TTL=0 TEMPEST_THROTTLE_MS=0
export CLICOLOR_FORCE=1
cd "$demo_dir"
"$repo_root/bin/agentdemo" -server "$repo_root/bin/tempestkeep" "$@"
cd "$repo_root"
cleanup
trap - EXIT INT TERM
printf '\n  Session closed. Temporary demo archive removed.\n'
