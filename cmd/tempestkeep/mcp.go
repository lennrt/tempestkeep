package main

import (
	"flag"
	"fmt"
	"os"

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
	readOnlyFlag := fs.Bool("read-only", false, "remove archive write tools (or env TEMPEST_READ_ONLY)")
	versionFlag := fs.Bool("version", false, "print the version and exit")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return usagef("mcp does not accept positional arguments")
	}
	if *versionFlag {
		fmt.Printf("tempestkeep mcp %s\n", version.String())
		return nil
	}

	ctx, stop := signalContext()
	defer stop()
	if err := config.LoadDotenv(ctx, ".env"); err != nil {
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
		ReadOnly: *readOnlyFlag || envReadOnly,
	})
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
