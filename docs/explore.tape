# Run `make demo-explore` from the repository root. Install vhs, ttyd, and
# ffmpeg first. The command seeds a temporary archive from the local
# deterministic mock API and opens the archive explorer.

Output docs/explore.gif

Set Shell bash
Set FontSize 15
Set Width 980
Set Height 680
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms

# Invisible setup: binaries on PATH, the mock API up, a temp archive seeded
# with everything the mock serves.
Hide
Type@0ms `export PATH="$PWD/bin:$PATH"; demo=$(mktemp -d) || exit 1; cd "$demo" || exit 1; demoapi -addr 127.0.0.1:8916 >/dev/null 2>&1 & api_pid=$!; export TEMPEST_API_BASE=http://127.0.0.1:8916 TEMPEST_TOKEN=demo TEMPEST_THROTTLE_MS=0 CLICOLOR_FORCE=1; export TEMPEST_DB="$PWD/tempest.sqlite"; tempest collect --no-backup >/dev/null 2>&1; clear`
Enter
Sleep 15
Show

Type "tempest explore   # the archive you own: day / week / month / year / records"
Enter
Sleep 5

# Day view first; scrub a couple of days back.
Left@800ms 2
Sleep 2.5

Type "w"
Sleep 3

Type "m"
Sleep 3

# Cycle the calendar heatmap's metric: high, low, rain, gust.
Tab@1200ms 3
Sleep 2.5

Type "y"
Sleep 3.5

Type "r"
Sleep 5.5

Type "q"
Sleep 1

# Invisible teardown.
Hide
Type `kill "$api_pid" 2>/dev/null; wait "$api_pid" 2>/dev/null || true; cd - >/dev/null; rm -f "$demo/tempest.sqlite" "$demo/tempest.sqlite-wal" "$demo/tempest.sqlite-shm" "$demo/tempest.sqlite-journal"; rmdir "$demo"`
Enter
