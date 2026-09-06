#!/bin/sh
# Exercise the same real MCP conversation as the README recording, without pauses.
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
transcript=$(mktemp "${TMPDIR:-/tmp}/tempestkeep-demo-check.XXXXXX")
trap 'rm -f -- "$transcript"' EXIT

# The runner must override ambient credentials, endpoints, and read-only mode.
NO_COLOR=1 TEMPEST_TOKEN=synthetic-unused-token TEMPEST_READ_ONLY=true \
	TEMPEST_API_BASE=https://example.invalid \
	"$repo_root/scripts/demo-agent.sh" -pace 0 -hold 0 >"$transcript"

for expected in \
	'29 tools / 2 resources / 3 prompts discovered' \
	'0 observations stored' \
	'has_more=true' \
	'reached the start of history' \
	'tool  daily_summary' \
	'tool  get_observations' \
	'No API token / --read-only' \
	'24 tools discovered; live and write tools absent' \
	'tool  wind_rose' \
	'MCP demo complete' \
	'Session closed. Temporary demo archive removed.'
do
	if ! grep -F "$expected" "$transcript" >/dev/null; then
		printf 'MCP demo did not demonstrate: %s\n' "$expected" >&2
		exit 1
	fi
done
printf 'MCP demo passed: discovery, resumable writes, chained queries, read-only reuse, cleanup.\n'
