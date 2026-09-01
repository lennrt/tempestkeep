# Run `make demo-agent` from the repository root. Install vhs, ttyd, and ffmpeg
# first. The command starts `tempestkeep mcp` over stdio and calls it through MCP. It
# uses the local deterministic mock API. The displayed text is scripted.

Output docs/agent.gif

Set Shell bash
Set FontSize 15
Set Width 980
Set Height 760
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms

# Invisible setup: binaries on PATH, the mock API up, a fresh archive path.
Hide
Type@0ms `export PATH="$PWD/bin:$PATH"; demo=$(mktemp -d) || exit 1; cd "$demo" || exit 1; demoapi -addr 127.0.0.1:8914 >/dev/null 2>&1 & api_pid=$!; export TEMPEST_API_BASE=http://127.0.0.1:8914 TEMPEST_TOKEN=demo TEMPEST_THROTTLE_MS=0 CLICOLOR_FORCE=1; export TEMPEST_DB="$PWD/tempest.sqlite"; clear`
Enter
Sleep 2
Show

Type "agentdemo   # a real MCP client driving tempestkeep mcp, like your agent does"
Enter

# The session: backfill_archive walks history back, then the analytics tools
# answer from the local copy. Wait for it to finish, then hold the last frame.
Sleep 42
Sleep 6

# Invisible teardown.
Hide
Type `kill "$api_pid" 2>/dev/null; wait "$api_pid" 2>/dev/null || true; cd - >/dev/null; rm -f "$demo/tempest.sqlite" "$demo/tempest.sqlite-wal" "$demo/tempest.sqlite-shm" "$demo/tempest.sqlite-journal"; rmdir "$demo"`
Enter
