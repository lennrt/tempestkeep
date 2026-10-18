package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/lennrt/tempestkeep/internal/mcpapp"
	"github.com/lennrt/tempestkeep/internal/version"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

// cmdMCP resolves command configuration, builds the API client at the command
// boundary, then gives the blocking MCP server a signal-aware context. MCP
// stdout is reserved for JSON-RPC after startup.
func cmdMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	describe(fs, "tempestkeep mcp: serve live and archived weather data over MCP stdio.",
		"tempestkeep mcp",
		"tempestkeep mcp --db ./tempest.sqlite",
		"tempestkeep mcp --read-only")
	dbFlag := fs.String("db", "", "path to the tempest.sqlite archive (or env TEMPEST_DB)")
	deviceFlag := fs.Int("device-id", 0, "default Tempest device id (or env TEMPEST_DEVICE_ID; auto-discovered if unset)")
	readOnlyFlag := fs.Bool("read-only", false, "remove archive write tools (or env TEMPEST_READ_ONLY)")
	versionFlag := fs.Bool("version", false, "print the version and exit")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	deviceSet := false
	fs.Visit(func(f *flag.Flag) { deviceSet = deviceSet || f.Name == "device-id" })
	if deviceSet && *deviceFlag <= 0 {
		return usagef("--device-id must be positive")
	}
	if *versionFlag {
		_, err := fmt.Printf("tempestkeep mcp %s\n", version.String())
		return err
	}

	ctx, stop := signalContext()
	defer stop()
	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}
	deviceID, err := resolveMCPDeviceID(*deviceFlag)
	if err != nil {
		return err
	}
	dbPath, err := config.ResolveDB(ctx, *dbFlag)
	if err != nil {
		return err
	}
	envReadOnly, err := readOnlyEnv()
	if err != nil {
		return err
	}
	var client *api.Client
	if token := os.Getenv("TEMPEST_TOKEN"); token != "" {
		if client, err = newAPIClient(token); err != nil {
			return err
		}
	}
	return mcpapp.Run(ctx, mcpapp.Options{
		Client:   client,
		DBPath:   dbPath,
		DeviceID: deviceID,
		ReadOnly: *readOnlyFlag || envReadOnly,
	})
}

// resolveMCPDeviceID receives either a validated positive flag or zero when the
// flag was absent. An explicit flag must not parse an overridden environment.
func resolveMCPDeviceID(flagValue int) (int, error) {
	if flagValue > 0 {
		return flagValue, nil
	}
	value := os.Getenv("TEMPEST_DEVICE_ID")
	if value == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(value)
	if err != nil || id < 0 {
		return 0, fmt.Errorf("TEMPEST_DEVICE_ID must be zero for discovery or a positive integer")
	}
	return id, nil
}

// readOnlyEnv parses TEMPEST_READ_ONLY. Unknown values are an error so a
// misspelling cannot enable writes.
func readOnlyEnv() (bool, error) {
	const key = "TEMPEST_READ_ONLY"
	value, err := config.ParseBool(os.Getenv(key))
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return value, nil
}
