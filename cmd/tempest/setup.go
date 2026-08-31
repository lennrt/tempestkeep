package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

// cmdSetup validates a token and writes a private environment file.
func cmdSetup(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	describe(fs, "Configure a token, archive location, and MCP command in an interactive terminal.\nThe command writes an owner-only environment file.",
		"tempest setup",
		"tempest setup --env ~/weather/.env")
	envPath := fs.String("env", ".env", "path of the env file to write")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if !isTTY(os.Stdin) || !isTTY(os.Stdout) {
		// The wizard is forms and spinners; under CI or a pipe huh would fail
		// with an opaque tty error. Fail fast and describe the manual path.
		return errors.New("setup is interactive and needs a terminal; to configure by hand, write a .env with\n" +
			"  TEMPEST_TOKEN=...      create one at tempestwx.com -> Settings -> Data Authorizations\n" +
			"  TEMPEST_DB=...         optional archive path (default ./tempest.sqlite)\n" +
			"  TEMPEST_DEVICE_ID=...  optional; auto-discovered when unset")
	}

	existing, err := readEnvFile(*envPath)
	if err != nil {
		return err
	}

	fmt.Println(lipgloss.NewStyle().Padding(1, 2).Render(splashArt()))
	fmt.Println(faint().Render("  This wizard writes " + *envPath + ", the one config file every\n" +
		"  TempestKeep tool (CLI, TUI, and MCP server) reads. Ctrl+C to abort.\n"))

	// --- 1. Token, validated against the live API -------------------------
	token := existing["TEMPEST_TOKEN"]
	var station *api.Station
	var deviceID int
	for {
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Personal access token").
				Description("Create one at tempestwx.com → Settings → Data Authorizations →\nCreate Token. It only needs read access to your own station.").
				Placeholder("20 hex chars, e.g. a1b2c3d4-…").
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("a token is required; the API allows nothing without one")
					}
					if strings.ContainsAny(s, " \t") {
						return fmt.Errorf("tokens never contain spaces; check the paste")
					}
					return nil
				}).
				Value(&token),
		))
		if err := form.Run(); err != nil {
			return err
		}
		token = strings.TrimSpace(token)

		var lookupErr error
		if err := spinner.New().Title("Checking the token against the WeatherFlow API…").
			Action(func() {
				ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				client, err := newAPIClient(token)
				if err != nil {
					lookupErr = err
					return
				}
				station, deviceID, lookupErr = client.FindTempestDevice(ctx)
			}).Run(); err != nil {
			return errors.New("token check display failed")
		}
		if lookupErr == nil {
			break
		}
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#E63946")).
			Render("  ✗ " + lookupErr.Error()))
		fmt.Println(faint().Render("  Let's try again: paste the token exactly as shown on tempestwx.com."))
		token = ""
	}
	fmt.Printf("  %s station %q (device %d)\n\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("#7FD1AE")).Render("✓ Found"),
		station.Name, deviceID)

	// --- 2. Where the archive lives ---------------------------------------
	// The recommended home is the per-user data directory: it survives whatever
	// directory the user happened to run the wizard from, and it's where an
	// always-growing database belongs. "This directory" stays on the menu for
	// people who keep the archive next to a project.
	cwd, err := os.Getwd()
	if err != nil {
		return setupIO("get current directory", err)
	}
	dataPath := defaultArchivePath()
	const (
		locData  = "data"
		locHere  = "here"
		locOther = "other"
		locNone  = "none"
	)
	choice := locData
	err = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Where should the SQLite archive live?").
			Description("The archive is your station's growing local history: records,\nsummaries, and wind roses are answered from this one file.").
			Options(
				huh.NewOption("Your user data folder (recommended)  →  "+displayPath(dataPath), locData),
				huh.NewOption("This directory  →  "+displayPath(filepath.Join(cwd, config.DefaultDB)), locHere),
				huh.NewOption("Somewhere else (custom path)", locOther),
				huh.NewOption("No archive (live conditions and forecast only)", locNone),
			).
			Value(&choice),
	)).Run()
	if err != nil {
		return err
	}

	dbPath := ""
	switch choice {
	case locData:
		dbPath = dataPath
	case locHere:
		dbPath = filepath.Join(cwd, config.DefaultDB)
	case locOther:
		dbPath, err = askDBPath()
		if err != nil {
			return err
		}
	}

	// --- 3. Confirm and write ---------------------------------------------
	updates := map[string]string{
		"TEMPEST_TOKEN":     token,
		"TEMPEST_DEVICE_ID": strconv.Itoa(deviceID),
	}
	if dbPath != "" {
		updates["TEMPEST_DB"] = dbPath
	}

	preview := "TEMPEST_TOKEN=" + maskToken(token) + "\nTEMPEST_DEVICE_ID=" + strconv.Itoa(deviceID)
	if dbPath != "" {
		preview += "\nTEMPEST_DB=" + dbPath
	}
	// Opting out of an archive must also drop a TEMPEST_DB left by an earlier
	// run, or later commands keep using the archive that the user declined.
	if _, had := existing["TEMPEST_DB"]; had && choice == locNone {
		delete(existing, "TEMPEST_DB")
		preview += "\n(removing the previous TEMPEST_DB: you chose no archive)"
	}
	write := true
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Write " + *envPath + "?").
			Description(preview + existingNote(*envPath, existing)).
			Affirmative("Write it").Negative("Abort").
			Value(&write),
	)).Run(); err != nil {
		return err
	}
	if !write {
		return fmt.Errorf("aborted; nothing written")
	}
	if err := writeEnvFile(*envPath, existing, updates); err != nil {
		return err
	}
	fmt.Printf("  ✓ Wrote %s\n\n", *envPath)

	printNextSteps(dbPath)
	return nil
}

// defaultArchivePath uses the platform data directory when one is available.
func defaultArchivePath() string {
	var dir string
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			dir = filepath.Join(d, "TempestKeep")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, "Library", "Application Support", "TempestKeep")
		}
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			dir = filepath.Join(d, "tempestkeep")
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".local", "share", "tempestkeep")
		}
	}
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, config.DefaultDB)
}

// displayPath shortens a home-directory prefix for display only.
func displayPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rest, ok := strings.CutPrefix(p, home); ok {
		return "~" + rest
	}
	return p
}

// askDBPath accepts a path only when its parent directory exists.
func askDBPath() (string, error) {
	title := "Path for the archive file"
	desc := "Full path ending in .sqlite on a local filesystem; the directory must already exist."
	path := ""
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(title).Description(desc).
			Placeholder("/path/to/local/weather/tempest.sqlite").
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return fmt.Errorf("a path is required")
				}
				if info, err := os.Stat(filepath.Dir(s)); err != nil || !info.IsDir() {
					return errors.New("the parent directory does not exist")
				}
				return nil
			}).
			Value(&path),
	)).Run()
	return strings.TrimSpace(path), err
}

// printNextSteps prints the next CLI and MCP commands.
func printNextSteps(dbPath string) {
	bold := lipgloss.NewStyle().Bold(true)
	cmd := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD23F"))

	fmt.Println(bold.Render("  Next steps"))
	if dbPath != "" {
		fmt.Println("    1. Build your local archive (resumable, safe to re-run):")
		fmt.Println(cmd.Render("         tempest collect"))
		fmt.Println("    2. Watch live conditions:")
		fmt.Println(cmd.Render("         tempest now"))
	} else {
		fmt.Println("    1. Watch live conditions:")
		fmt.Println(cmd.Render("         tempest now"))
	}

	// The archive branch above prints two steps, the no-archive branch one, so
	// number this step to follow whichever ran (no skipped "2.").
	mcpStep := 2
	if dbPath != "" {
		mcpStep = 3
	}
	fmt.Printf("    %d. Give the data to Claude Code (or point any MCP client at tempest-mcp):\n", mcpStep)
	fmt.Println(faint().Render("       Set TEMPEST_TOKEN in the MCP client's private environment first."))
	for _, line := range mcpRegistrationCommand(dbPath) {
		fmt.Println(cmd.Render("         " + line))
	}
	if dbPath != "" {
		fmt.Println(faint().Render("       No archive yet? Ask the agent to run backfill_archive; it will\n" +
			"       build the history itself, one resumable batch at a time."))
	}
	fmt.Println()
}

func mcpRegistrationCommand(dbPath string) []string {
	if runtime.GOOS == "windows" {
		line := "claude mcp add tempest -- tempest-mcp"
		if dbPath != "" {
			line += " --db " + commandArg(dbPath)
		}
		return []string{line}
	}
	lines := []string{
		"claude mcp add tempest \\",
		"  -- tempest-mcp",
	}
	if dbPath != "" {
		lines[1] += " \\"
		lines = append(lines, "  --db "+commandArg(dbPath))
	}
	return lines
}

// commandArg quotes one path for the platform's common interactive shell.
func commandArg(value string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// maskToken hides the middle of a token for confirmation screens.
func maskToken(t string) string {
	if len(t) <= 8 {
		return strings.Repeat("*", len(t))
	}
	return t[:4] + strings.Repeat("*", len(t)-8) + t[len(t)-4:]
}

// existingNote describes what happens to an existing env file, if any.
func existingNote(path string, existing map[string]string) string {
	if len(existing) == 0 {
		return ""
	}
	return fmt.Sprintf("\n\n%s exists: these keys are updated, everything else is kept.", path)
}

// readEnvFile reads one bounded, private, regular environment file.
func readEnvFile(path string) (values map[string]string, retErr error) {
	pathInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, setupIO("inspect environment file", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: environment file must be a regular file, not a symlink", config.ErrInvalidConfig)
	}
	if pathInfo.Size() > config.MaxDotenvBytes {
		return nil, fmt.Errorf("%w: environment file exceeds %d bytes", config.ErrInvalidConfig, config.MaxDotenvBytes)
	}
	if runtime.GOOS != "windows" && pathInfo.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: environment file permissions must not grant group or other access", config.ErrInvalidConfig)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, setupIO("open environment file", err)
	}
	defer func() { retErr = errors.Join(retErr, setupIO("close environment file", f.Close())) }()
	info, err := f.Stat()
	if err != nil {
		return nil, setupIO("inspect open environment file", err)
	}
	if !os.SameFile(pathInfo, info) {
		return nil, fmt.Errorf("%w: environment file changed while it was opened", config.ErrInvalidConfig)
	}
	data, err := io.ReadAll(io.LimitReader(f, config.MaxDotenvBytes+1))
	if err != nil {
		return nil, setupIO("read environment file", err)
	}
	if len(data) > config.MaxDotenvBytes {
		return nil, fmt.Errorf("%w: environment file exceeds %d bytes", config.ErrInvalidConfig, config.MaxDotenvBytes)
	}
	return config.ParseDotenv(data)
}

// writeEnvFile keeps unrelated keys and writes TempestKeep keys first.
func writeEnvFile(path string, existing, updates map[string]string) error {
	merged := map[string]string{}
	maps.Copy(merged, existing)
	maps.Copy(merged, updates)

	var b strings.Builder
	b.WriteString("# TempestKeep configuration, written by `tempest setup`.\n")
	ours := []string{"TEMPEST_TOKEN", "TEMPEST_DB", "TEMPEST_DEVICE_ID"}
	for _, k := range ours {
		if v, ok := merged[k]; ok {
			formatted, err := config.FormatDotenvValue(v)
			if err != nil {
				return fmt.Errorf("format environment value: %w", err)
			}
			fmt.Fprintf(&b, "%s=%s\n", k, formatted)
			delete(merged, k)
		}
	}
	rest := make([]string, 0, len(merged))
	for k := range merged {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		formatted, err := config.FormatDotenvValue(merged[k])
		if err != nil {
			return fmt.Errorf("format environment value: %w", err)
		}
		fmt.Fprintf(&b, "%s=%s\n", k, formatted)
	}
	return writePrivateFile(path, []byte(b.String()))
}

// writePrivateFile atomically replaces a regular file with mode 0600.
func writePrivateFile(path string, data []byte) (retErr error) {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("refuse to replace a non-regular environment file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return setupIO("inspect environment file", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return setupIO("create temporary environment file", err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if retErr != nil && !renamed {
			retErr = errors.Join(retErr, setupIO("remove temporary environment file", os.Remove(tmpName)))
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return errors.Join(setupIO("set environment file permissions", err), setupIO("close environment file", tmp.Close()))
	}
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(setupIO("write environment file", err), setupIO("close environment file", tmp.Close()))
	}
	if err := tmp.Sync(); err != nil {
		return errors.Join(setupIO("sync environment file", err), setupIO("close environment file", tmp.Close()))
	}
	if err := tmp.Close(); err != nil {
		return setupIO("close environment file", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return setupIO("replace environment file", err)
	}
	renamed = true
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(dir)
	if err != nil {
		return setupIO("open environment directory", err)
	}
	return errors.Join(
		setupIO("sync environment directory", directory.Sync()),
		setupIO("close environment directory", directory.Close()),
	)
}

func setupIO(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", config.ErrConfigIO, action)
}
