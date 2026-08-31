# Run `make demo-setup` from the repository root. Install vhs, ttyd, and ffmpeg
# first. The command uses the local deterministic mock API. The setup writes
# only inside a temporary directory.

Output docs/setup.gif

Set Shell bash
Set FontSize 15
Set Width 980
Set Height 600
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms

# Invisible setup: binaries on PATH, the mock API up, and a throwaway demo dir.
# Never write the fake demo token outside the temporary directory.
Hide
Type@0ms `export PATH="$PWD/bin:$PATH"; demo=$(mktemp -d /tmp/tk.XXXXXX) || exit 1; cd "$demo" || exit 1; demoapi -addr 127.0.0.1:8912 >/dev/null 2>&1 & api_pid=$!; export TEMPEST_API_BASE=http://127.0.0.1:8912; clear`
Enter
Sleep 2
Show

Type "tempest setup   # first run? the wizard takes it from here"
Enter
Sleep 4

# Paste the (fake) personal access token, masked as it's typed.
Type@40ms "demo-token-only"
Sleep 1.5
Enter
Sleep 5

# Archive location: choose the current temporary directory.
Sleep 2
Down
Enter
Sleep 2.5

# Confirm writing .env.
Enter
Sleep 3

# The wizard prints next steps, including the exact `claude mcp add` line.
Sleep 8

# Invisible teardown.
Hide
Type `kill "$api_pid" 2>/dev/null; wait "$api_pid" 2>/dev/null || true; cd - >/dev/null; rm -f "$demo/.env"; rmdir "$demo"`
Enter
