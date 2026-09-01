# Run `make demo` from the repository root. Install vhs, ttyd, and ffmpeg first.
# The command builds the binaries and uses the local deterministic mock API. It
# does not need a token or station. CI can also record the artifact.

Output docs/demo.gif

Set Shell bash
Set FontSize 15
Set Width 980
Set Height 640
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms

# Invisible setup: binaries on PATH, the mock API up, and the JSON-RPC lines the
# MCP listing pipes in staged in a fresh temp dir.
Hide
Type@0ms `export PATH="$PWD/bin:$PATH"; demo=$(mktemp -d) || exit 1; cd "$demo" || exit 1; demoapi -addr 127.0.0.1:8910 >/dev/null 2>&1 & api_pid=$!; export TEMPEST_API_BASE=http://127.0.0.1:8910 TEMPEST_TOKEN=demo TEMPEST_THROTTLE_MS=0 CLICOLOR_FORCE=1; printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"demo","version":"0"}}}' '{"jsonrpc":"2.0","method":"notifications/initialized"}' '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' > mcp.jsonl; clear`
Enter
Sleep 3
Show

Type "# TempestKeep: a local weather archive with CLI and MCP access"
Enter
Sleep 2.5

Type "tempestkeep collect --no-backup   # first run: collect available history"
Enter
Sleep 9

Type "tempestkeep now --once   # live conditions, wttr.in-style"
Enter
Sleep 6

Type "# the same archive, served to agents over MCP: one binary, 29 tools"
Enter
Sleep 1
Type `cat mcp.jsonl | tempestkeep mcp 2>/dev/null | grep -oE '"name":"[a-z_]+"' | cut -d'"' -f4 | pr -3 -t -w 66`
Enter
Sleep 5

Type "# agents keep it fresh themselves: backfill_archive -> sync_archive ->"
Enter
Type "# records, this_day_in_history, wind_rose, period_summary... all offline"
Enter
Sleep 5

# Invisible teardown.
Hide
Type `kill "$api_pid" 2>/dev/null; wait "$api_pid" 2>/dev/null || true; cd - >/dev/null; rm -f "$demo/mcp.jsonl" "$demo/tempest.sqlite" "$demo/tempest.sqlite-wal" "$demo/tempest.sqlite-shm" "$demo/tempest.sqlite-journal"; rmdir "$demo"`
Enter
