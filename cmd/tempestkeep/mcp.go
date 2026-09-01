package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/lennrt/tempestkeep/internal/mcpapp"
	"github.com/lennrt/tempestkeep/internal/version"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
)

// cmdMCP resolves command configuration, then gives the blocking MCP server an
// interrupt-aware context. MCP stdout is reserved for JSON-RPC after startup.
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
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
	return mcpapp.Run(ctx, mcpapp.Options{
		Token:    os.Getenv("TEMPEST_TOKEN"),
		DBPath:   dbPath,
		ReadOnly: *readOnlyFlag || envReadOnly,
	})
}

// readOnlyEnv rejects unknown values so a misspelling cannot enable writes.
func readOnlyEnv() (bool, error) {
	const key = "TEMPEST_READ_ONLY"
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true, nil
	case "", "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("%s must be a boolean (1/0, true/false, yes/no, on/off)", key)
}
