// Command agentdemo records docs/agent.tape. It starts `tempestkeep mcp` over stdio
// and calls its tools against the local mock API. The text is scripted. Tool
// results come from the MCP server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	styleYou   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD23F"))
	styleAgent = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5B9BD5"))
	styleTool  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7FD1AE"))
	styleDim   = lipgloss.NewStyle().Faint(true)

	// pace is the pause between printed lines, so the recording reads like a
	// session rather than a dump. Overridable for fast local runs.
	pace   = flag.Duration("pace", 450*time.Millisecond, "pause between lines")
	hold   = flag.Duration("hold", 3*time.Second, "pause after each scene")
	scenes = flag.Bool("scenes", false, "clear the screen between recording scenes")
)

const maxPace = 5 * time.Second

func main() {
	if err := os.Setenv("CLICOLOR_FORCE", "1"); err != nil {
		log.Fatal(err)
	}
	server := flag.String("server", "tempestkeep", "path to the tempestkeep binary")
	flag.Parse()
	if *pace < 0 || *pace > maxPace || *hold < 0 || *hold > 10*time.Second {
		log.Fatal("pace must be 0..5s and hold must be 0..10s")
	}
	if *server == "" || len(*server) > 4096 || strings.IndexByte(*server, 0) >= 0 {
		log.Fatal("server path must contain 1..4096 bytes and no NUL")
	}

	if err := run(*server); err != nil {
		log.Fatal(err)
	}
}

func run(server string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	scene("01 / DISCOVER", "One MCP connection. Tools, resources, and prompts.")
	cs, err := connect(ctx, server, false)
	if err != nil {
		return err
	}
	defer func() {
		if cs != nil {
			err = errors.Join(err, cs.Close())
		}
	}()
	init := cs.InitializeResult()
	if init.ServerInfo == nil {
		return errors.New("server returned no identity")
	}
	say(styleTool.Render("  initialize") + "  " + init.ServerInfo.Name + " " + init.ServerInfo.Version)
	say(styleDim.Render("  Go MCP SDK  ->  stdio / JSON-RPC  ->  tempestkeep mcp"))
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	resources, err := cs.ListResources(ctx, nil)
	if err != nil {
		return err
	}
	prompts, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		return err
	}
	result(fmt.Sprintf("%d tools / %d resources / %d prompts discovered", len(tools.Tools), len(resources.Resources), len(prompts.Prompts)))
	say("")
	say("  Live weather      current_conditions / forecast")
	say("  Local history     daily_summary / wind_rose / query_sql")
	say("  Archive writes    backfill_archive / sync_archive")
	time.Sleep(*hold)

	scene("02 / BUILD", "The client builds a resumable local archive.")
	if err := actBuildArchive(ctx, cs); err != nil {
		return err
	}
	time.Sleep(*hold)
	scene("03 / ASK", "Chain tool results into the next question.")
	if err := actWindiestDay(ctx, cs); err != nil {
		return err
	}
	time.Sleep(*hold)

	if err := cs.Close(); err != nil {
		return err
	}
	cs = nil
	scene("04 / REUSE", "Restart with no API token. Keep asking the archive.")
	offline, err := connect(ctx, server, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, offline.Close()) }()
	tools, err = offline.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	for _, tool := range tools.Tools {
		if tool.Name == "backfill_archive" || tool.Name == "sync_archive" || tool.Name == "forecast" {
			return errors.New("archive-only session exposed a live or write tool")
		}
	}
	say(styleDim.Render("  No API token / --read-only / same SQLite archive"))
	result(fmt.Sprintf("%d tools discovered; live and write tools absent", len(tools.Tools)))
	if err := actWindRose(ctx, offline); err != nil {
		return err
	}
	say("")
	say(styleTool.Render("  MCP demo complete") + styleDim.Render("  /  your agent, your station, your archive"))
	time.Sleep(*hold)
	return nil
}

func connect(ctx context.Context, server string, readOnly bool) (*mcp.ClientSession, error) {
	args := []string{"mcp"}
	if readOnly {
		args = append(args, "--read-only")
	}
	cmd := exec.CommandContext(ctx, server, args...)
	cmd.Stderr = io.Discard
	if readOnly {
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "TEMPEST_TOKEN=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "TEMPEST_TOKEN=")
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "agentdemo", Version: "0"}, nil)
	return client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second}, nil)
}

func scene(number, description string) {
	if *scenes {
		fmt.Print("\x1b[2J\x1b[H")
	}
	say(styleTool.Bold(true).Render("  TEMPESTKEEP") + styleDim.Render("  /  WEATHER CONTEXT FOR AGENTS"))
	say(styleDim.Render("  Scripted client / synthetic weather / real MCP calls"))
	say("")
	say(styleAgent.Render("  "+number) + "  " + description)
	say(styleDim.Render("  " + strings.Repeat("─", 76)))
}

// ---- act 1: the agent builds its own archive ----------------------------------

func actBuildArchive(ctx context.Context, cs *mcp.ClientSession) error {
	you("Build my station's local archive. Resume until all history is stored.")

	var status struct {
		Observations int64  `json:"observations"`
		Note         string `json:"note"`
	}
	if err := call(ctx, cs, "archive_status", nil, &status); err != nil {
		return err
	}
	result(fmt.Sprintf("%d observations stored", status.Observations))

	// Walk history back until the API runs out: the resumable loop an agent
	// drives by re-calling while has_more is true.
	total := int64(0)
	for {
		var out struct {
			RowsAdded    int    `json:"rows_added"`
			HasMore      bool   `json:"has_more"`
			Observations int64  `json:"observations"`
			Coverage     string `json:"coverage"`
		}
		if err := call(ctx, cs, "backfill_archive", map[string]any{"max_days": 30}, &out); err != nil {
			return err
		}
		total = out.Observations
		switch {
		case out.HasMore:
			result(fmt.Sprintf("+%s rows · has_more=true → calling again", comma(int64(out.RowsAdded))))
		default:
			result(fmt.Sprintf("+%s rows · reached the start of history", comma(int64(out.RowsAdded))))
		}
		if !out.HasMore {
			break
		}
	}

	if total == 0 {
		return errors.New("demo backfill returned no observations")
	}
	agent(fmt.Sprintf("%s one-minute observations stored in SQLite. Historical questions can now be answered from the local archive.", comma(total)))
	return nil
}

// ---- act 2: windiest day, gusts or sustained? ---------------------------------

func actWindiestDay(ctx context.Context, cs *mcp.ClientSession) error {
	you("Which day had the strongest gust in the last 30 days? Compare its hourly wind.")

	var daily struct {
		Days []struct {
			Day         string   `json:"day"`
			PeakGustMph *float64 `json:"peak_gust_mph"`
		} `json:"days"`
	}
	if err := call(ctx, cs, "daily_summary", map[string]any{"days": 30}, &daily); err != nil {
		return err
	}
	bestDay, bestGust := "", 0.0
	for _, d := range daily.Days {
		if d.PeakGustMph != nil && *d.PeakGustMph > bestGust {
			bestDay, bestGust = d.Day, *d.PeakGustMph
		}
	}
	if bestDay == "" {
		agent("I don't see any wind data in the archive yet.")
		return nil
	}
	result(fmt.Sprintf("peak gust %.0f mph on %s", bestGust, bestDay))

	// Drill into that day for the sustained picture, the tool chaining an agent
	// does naturally: summary first, then the series behind it.
	var series struct {
		Points []struct {
			WindMph *float64 `json:"wind_mph"`
		} `json:"points"`
	}
	args := map[string]any{"start": bestDay, "end": bestDay, "bucket_minutes": 60}
	if err := call(ctx, cs, "get_observations", args, &series); err != nil {
		return err
	}
	sustained := 0.0
	for _, p := range series.Points {
		if p.WindMph != nil && *p.WindMph > sustained {
			sustained = *p.WindMph
		}
	}
	result(fmt.Sprintf("highest hourly mean: %.0f mph", sustained))

	agent(fmt.Sprintf("%s had the strongest gust: %.0f mph. Its highest hourly mean was %.0f mph. The daily summary picked the day; hourly observations supplied the comparison.",
		shortDate(bestDay), bestGust, sustained))
	return nil
}

// ---- act 3: the wind rose -------------------------------------------------------

func actWindRose(ctx context.Context, cs *mcp.ClientSession) error {
	you("Using only the local archive, where does the wind usually come from?")

	var rose struct {
		Sectors []struct {
			Sector string   `json:"sector"`
			Pct    float64  `json:"pct"`
			AvgMph *float64 `json:"avg_mph"`
		} `json:"sectors"`
		CalmPct float64 `json:"calm_pct"`
	}
	if err := call(ctx, cs, "wind_rose", nil, &rose); err != nil {
		return err
	}
	sort.Slice(rose.Sectors, func(i, j int) bool { return rose.Sectors[i].Pct > rose.Sectors[j].Pct })
	if len(rose.Sectors) < 2 || rose.Sectors[0].Pct == 0 {
		agent("There's no directional wind data in the archive yet.")
		return nil
	}
	top, second := rose.Sectors[0], rose.Sectors[1]
	result(fmt.Sprintf("%s %.0f%% · %s %.0f%% · calm %.0f%%", top.Sector, top.Pct, second.Sector, second.Pct, rose.CalmPct))

	answer := fmt.Sprintf("Mostly %s: %.0f%% of non-calm samples, with %s next at %.0f%%.", top.Sector, top.Pct, second.Sector, second.Pct)
	if top.AvgMph != nil {
		answer += fmt.Sprintf(" It averages %.0f mph from that direction, and the air is calm %.0f%% of the time.", *top.AvgMph, rose.CalmPct)
	}
	agent(answer)
	return nil
}

// ---- MCP plumbing ---------------------------------------------------------------

// call invokes one tool, prints the call line, and decodes the structured
// result into out via JSON round-trip (the server publishes output schemas, so
// the shape is stable).
func call(ctx context.Context, cs *mcp.ClientSession, name string, args map[string]any, out any) error {
	line := "  " + styleDim.Render("tool  ") + styleTool.Render(name)
	if len(args) > 0 {
		if j, err := json.Marshal(args); err == nil {
			line += "\n    " + styleDim.Render(string(j))
		}
	}
	say(line)

	if args == nil {
		args = map[string]any{}
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if res.IsError {
		return fmt.Errorf("%s: %s", name, textOf(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return fmt.Errorf("%s: marshal structured content: %w", name, err)
	}
	return json.Unmarshal(raw, out)
}

func textOf(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			return t.Text
		}
	}
	return "tool error"
}

// ---- presentation ----------------------------------------------------------------

func say(s string) {
	fmt.Println(s)
	time.Sleep(*pace)
}

func you(q string) {
	say("")
	say(styleYou.Render("  ask   ") + wrap(q, 8))
	say("")
}

func agent(a string) {
	say("")
	say(styleAgent.Render("  reply ") + wrap(a, 8))
}

func result(s string) {
	say("    " + styleDim.Render("↳ "+s))
}

// wrap hard-wraps s to the demo's line width with a hanging indent.
func wrap(s string, indent int) string {
	const width = 76
	var (
		b    strings.Builder
		line = indent
	)
	for i, word := range strings.Fields(s) {
		if i > 0 {
			if line+1+len(word) > width {
				b.WriteString("\n" + strings.Repeat(" ", indent))
				line = indent
			} else {
				b.WriteString(" ")
				line++
			}
		}
		b.WriteString(word)
		line += len(word)
	}
	return b.String()
}

// comma renders 64801 as "64,801".
func comma(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	return s + "," + strings.Join(parts, ",")
}

// shortDate reformats YYYY-MM-DD as "Jul 10"; anything else passes through.
func shortDate(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("Jan 2")
	}
	return s
}
