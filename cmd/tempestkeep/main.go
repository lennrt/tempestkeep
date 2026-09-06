// Command tempestkeep reads a WeatherFlow Tempest station and maintains a local
// SQLite archive.
//
//	tempestkeep setup               configure a token, archive, and MCP client
//	tempestkeep now                 show current conditions and the forecast
//	tempestkeep now --once          render one frame and exit (pipe-friendly)
//	tempestkeep explore             browse archived days, weeks, months, years, and records
//	tempestkeep stats               print a climate summary of the archive (text or JSON)
//	tempestkeep collect             build or refresh the local archive (sync or backfill)
//	tempestkeep export              stream a date range to CSV or JSON Lines on stdout
//	tempestkeep list-devices        show the stations and devices the token can access
//	tempestkeep mcp                 serve live and archived data over MCP stdio
//	tempestkeep version             print the installed version
//	tempestkeep help [command]      show usage
//
// Read TEMPEST_TOKEN from the process environment or a private .env file.
// Other settings can also come from flags; a flag overrides the environment,
// and the environment overrides .env.
//
// Exit status is 0 on success, 1 for a runtime failure, and 2 for a usage
// error. Machine-readable commands write data to stdout and diagnostics to
// stderr.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"

	"github.com/lennrt/tempestkeep/internal/version"
	"github.com/mattn/go-isatty"
)

// Process exit statuses. They are part of the command-line contract and are
// documented in cmd/tempestkeep/README.md.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches one invocation and returns the exit status. It is the only
// function that maps errors to statuses and it never calls os.Exit. The given
// streams receive built-in output and dispatch diagnostics; command handlers
// retain their own output streams.
func run(args []string, stdout, stderr io.Writer) int {
	// --no-color may appear anywhere on the line; strip it before dispatch and
	// set NO_COLOR so the lipgloss/termenv default renderer drops ANSI. This sits
	// alongside the color handling we already inherit: a non-TTY stdout, an
	// externally-set NO_COLOR, and TERM=dumb all disable color on their own.
	args, noColor := stripNoColor(args)
	if noColor {
		if err := os.Setenv("NO_COLOR", "1"); err != nil {
			_, _ = fmt.Fprintf(stderr, "tempestkeep: disable color: %v\n", err)
			return exitFailure
		}
	}
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}

	err := dispatch(args, stdout, stderr)
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, flag.ErrHelp):
		return exitOK // -h already printed the flag usage; asking for help is not a failure
	case errors.Is(err, errUsageShown):
		return exitUsage // the parse error and usage were already written
	default:
		_, _ = fmt.Fprintf(stderr, "tempestkeep: %v\n", err)
		if isUsageErr(err) {
			return exitUsage // bad invocation, not a runtime failure
		}
		return exitFailure
	}
}

// dispatch runs the named command. Built-in commands (version, help) write to
// the given streams; registered commands own their own output.
func dispatch(args []string, stdout, stderr io.Writer) error {
	switch cmd := args[0]; cmd {
	case "version", "-v", "--version":
		_, err := fmt.Fprintf(stdout, "tempestkeep %s\n", version.String())
		return err
	case "help", "-h", "--help":
		if len(args) > 1 {
			// `tempestkeep help <cmd>` is `tempestkeep <cmd> -h`.
			handler, ok := commands[args[1]]
			if !ok {
				return unknownCommand(args[1], stderr)
			}
			return handler([]string{"-h"})
		}
		usage(stdout) // an explicit help request is a success; write it to stdout
		return nil
	default:
		handler, ok := commands[cmd]
		if !ok {
			return unknownCommand(cmd, stderr)
		}
		return handler(args[1:])
	}
}

// commands is the single command index used by dispatch, help, and suggestions.
var commands = map[string]func([]string) error{
	"setup":        cmdSetup,
	"now":          cmdNow,
	"explore":      cmdExplore,
	"collect":      cmdCollect,
	"export":       cmdExport,
	"stats":        cmdStats,
	"list-devices": cmdListDevices,
	"mcp":          cmdMCP,
}

// unknownCommand reports an unknown command as a usage error (exit status 2).
// A near match is suggested but never run. Without a near match the full
// usage page is written to stderr and errUsageShown tells run not to print
// anything further.
func unknownCommand(cmd string, stderr io.Writer) error {
	if s := closestCommand(cmd); s != "" {
		return usagef("unknown command %q (did you mean %q? See 'tempestkeep help')", cmd, s)
	}
	_, _ = fmt.Fprintf(stderr, "tempestkeep: unknown command %q\n\n", cmd)
	usage(stderr)
	return errUsageShown
}

// closestCommand returns a command within edit distance 2.
func closestCommand(cmd string) string {
	best, bestDist := "", 3
	for name := range commands {
		if d := editDistance(cmd, name); d < bestDist {
			best, bestDist = name, d
		}
	}
	return best
}

// editDistance computes Levenshtein distance for short command names.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// usageErr marks an invalid invocation. The process exits with status 2.
type usageErr struct{ err error }

func (u usageErr) Error() string { return u.err.Error() }
func (u usageErr) Unwrap() error { return u.err }

func usagef(format string, a ...any) error { return usageErr{fmt.Errorf(format, a...)} }

func isUsageErr(err error) bool {
	var u usageErr
	return errors.As(err, &u)
}

// errUsageShown tells run that the usage text was already written, so it
// exits with status 2 without printing the error a second time.
var errUsageShown = errors.New("usage already shown")

// parseFlags writes help to stdout and parse errors to stderr.
func parseFlags(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard) // suppress the flag package's own printing; we route below
	err := fs.Parse(args)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, flag.ErrHelp):
		fs.SetOutput(os.Stdout)
		fs.Usage()
		return flag.ErrHelp
	default:
		fs.SetOutput(os.Stderr)
		fmt.Fprintf(os.Stderr, "tempestkeep %s: %v\n\n", fs.Name(), err)
		fs.Usage()
		return errUsageShown
	}
}

// isTTY recognizes native and Cygwin/MSYS terminals.
func isTTY(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// signalContext returns a context that is canceled by SIGINT or SIGTERM. Long
// running commands use it so that Ctrl-C in a terminal and a service manager
// or MCP client stopping the process both unwind through the same path and
// close the archive cleanly. Callers must defer the returned stop function
// to release signal registrations when the command returns.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// describe adds a summary and examples to a command's flag help.
func describe(fs *flag.FlagSet, oneLine string, examples ...string) {
	fs.Usage = func() {
		out := textOutput{writer: fs.Output()}
		out.println(oneLine)
		if len(examples) > 0 {
			out.print("\nExamples:\n")
			for _, ex := range examples {
				out.printf("  %s\n", ex)
			}
		}
		out.print("\nFlags:\n")
		printFlagDefaults(&out, fs)
	}
}

type textOutput struct {
	writer io.Writer
	err    error
}

func (o *textOutput) print(values ...any) {
	if o.err == nil {
		_, o.err = fmt.Fprint(o.writer, values...)
	}
}

func (o *textOutput) printf(format string, values ...any) {
	if o.err == nil {
		_, o.err = fmt.Fprintf(o.writer, format, values...)
	}
}

func (o *textOutput) println(values ...any) {
	if o.err == nil {
		_, o.err = fmt.Fprintln(o.writer, values...)
	}
}

func printFlagDefaults(out *textOutput, fs *flag.FlagSet) {
	type row struct{ left, description string }
	var rows []row
	maxWidth := 0
	fs.VisitAll(func(f *flag.Flag) {
		prefix := "  --"
		if len(f.Name) == 1 {
			prefix = "  -"
		}
		left := prefix + f.Name
		value := reflect.ValueOf(f.Value)
		isBool := false
		if getter, ok := f.Value.(flag.Getter); ok {
			_, isBool = getter.Get().(bool)
		}
		if !isBool {
			kind := "VALUE"
			if value.IsValid() && strings.Contains(strings.ToLower(value.Type().String()), "int") {
				kind = "N"
			}
			left += " " + kind
		}
		description := f.Usage
		if f.DefValue != "" && f.DefValue != "0" && f.DefValue != "false" {
			description += " (default " + f.DefValue + ")"
		}
		rows = append(rows, row{left: left, description: description})
		maxWidth = max(maxWidth, len(left))
	})
	const helpWidth = 96
	descriptionWidth := max(24, helpWidth-maxWidth-2)
	for _, r := range rows {
		lines := wrapWords(r.description, descriptionWidth)
		out.printf("%-*s  %s\n", maxWidth, r.left, lines[0])
		for _, line := range lines[1:] {
			out.printf("%-*s  %s\n", maxWidth, "", line)
		}
	}
}

func wrapWords(value string, width int) []string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		if len(lines[last])+1+len(word) <= width {
			lines[last] += " " + word
			continue
		}
		lines = append(lines, word)
	}
	return lines
}

// stripNoColor removes the global color flag before command dispatch.
func stripNoColor(args []string) (rest []string, noColor bool) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		if a == "--no-color" || a == "-no-color" {
			noColor = true
			continue
		}
		rest = append(rest, a)
	}
	return rest, noColor
}

func usage(w io.Writer) {
	out := textOutput{writer: w}
	out.print(`TempestKeep
Read live Tempest data and maintain one local archive.

Usage:
  tempestkeep <command> [flags]

Start
  setup          configure a token, archive, and MCP client
  now            show current conditions and the five-day outlook

Archive
  collect        build or refresh local history
  explore        browse day, week, month, year, and record views
  stats          print a climate summary (text or JSON)
  export         stream observations as CSV or JSON Lines

Utilities
  list-devices   list stations and device IDs visible to the token
  mcp            serve live and archived data over MCP stdio
  version        print the installed version
  help           show this page or help for one command

Run 'tempestkeep help <command>' for flags and examples.

Global flag:
  --no-color     disable ANSI color (also honors NO_COLOR and TERM=dumb)

Configuration (flags override env override .env):
  TEMPEST_TOKEN      WeatherFlow personal access token
  TEMPEST_DB         archive path (default ./tempest.sqlite)
  TEMPEST_DEVICE_ID  device to collect (auto-discovered when unset)

`)
}
