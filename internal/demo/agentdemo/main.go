// Command agentdemo records docs/agent.tape. It starts tempest-mcp over stdio
// and calls its tools against the local mock API. The text is scripted. Tool
// results come from the MCP server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	pace = flag.Duration("pace", 450*time.Millisecond, "pause between lines")
)

const maxPace = 5 * time.Second

func main() {
	if err := os.Setenv("CLICOLOR_FORCE", "1"); err != nil {
		log.Fatal(err)
	}
	server := flag.String("server", "tempest-mcp", "path to the tempest-mcp binary")
	flag.Parse()
	if *pace < 0 || *pace > maxPace {
		log.Fatalf("pace must be between 0 and %s", maxPace)
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

	client := mcp.NewClient(&mcp.Implementation{Name: "agentdemo", Version: "0"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, server)}, nil)
	if err != nil {
		return errors.New("connect to MCP server failed")
	}
	defer func() { err = errors.Join(err, cs.Close()) }()

	say(styleDim.Render("· scripted conversation; every tool call below runs for real,"))
	say(styleDim.Render("  over MCP stdio, against the server your agent would use ·"))
	say("")

	if err := actBuildArchive(ctx, cs); err != nil {
		return err
	}
	if err := actWindiestDay(ctx, cs); err != nil {
		return err
	}
	if err := actWindRose(ctx, cs); err != nil {
		return err
	}
	return nil
}

// ---- act 1: the agent builds its own archive ----------------------------------

func actBuildArchive(ctx context.Context, cs *mcp.ClientSession) error {
	you("Build a local archive of my station's history, then tell me something surprising.")

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

	var rec struct {
		HottestF     *float64 `json:"hottest_f"`
		HottestTime  string   `json:"hottest_time"`
		PeakGustMph  *float64 `json:"peak_gust_mph"`
		PeakGustTime string   `json:"peak_gust_time"`
		WettestDay   string   `json:"wettest_day"`
		WettestDayIn *float64 `json:"wettest_day_in"`
		TotalStrikes *float64 `json:"total_lightning_strikes"`
	}
	if err := call(ctx, cs, "records", nil, &rec); err != nil {
		return err
	}
	result("all-time records computed from the local copy")

	answer := fmt.Sprintf("Your archive now holds %s one-minute observations; history is answered locally from here on.", comma(total))
	if rec.PeakGustMph != nil && rec.WettestDay != "" && rec.TotalStrikes != nil && *rec.TotalStrikes > 0 {
		answer += fmt.Sprintf(" The surprise: your wettest day (%s, %.2f in) also brought %.0f lightning strikes and your peak gust of %.0f mph. One evening thundershower owns most of your extremes.",
			shortDate(rec.WettestDay), *rec.WettestDayIn, *rec.TotalStrikes, *rec.PeakGustMph)
	} else if rec.HottestF != nil {
		answer += fmt.Sprintf(" All-time high so far: %.1f°F (%s).", *rec.HottestF, rec.HottestTime)
	}
	agent(answer)
	return nil
}

// ---- act 2: windiest day, gusts or sustained? ---------------------------------

func actWindiestDay(ctx context.Context, cs *mcp.ClientSession) error {
	you("What was the windiest day this month: gusts, or sustained wind?")

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
	result(fmt.Sprintf("hourly sustained wind topped out at %.0f mph", sustained))

	agent(fmt.Sprintf("%s was the windiest: gusts hit %.0f mph while sustained wind never passed %.0f mph, so it was gusty rather than steadily windy.",
		shortDate(bestDay), bestGust, sustained))
	return nil
}

// ---- act 3: the wind rose -------------------------------------------------------

func actWindRose(ctx context.Context, cs *mcp.ClientSession) error {
	you("Where does my wind usually come from?")

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

	answer := fmt.Sprintf("Mostly %s: %.0f%% of observed wind, with %s next at %.0f%%.", top.Sector, top.Pct, second.Sector, second.Pct)
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
	line := "  " + styleDim.Render("⚙ calling ") + styleTool.Render(name)
	if len(args) > 0 {
		if j, err := json.Marshal(args); err == nil {
			line += " " + styleDim.Render(string(j))
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
	say(styleYou.Render("❯ you   ") + wrap(q, 8))
	say("")
}

func agent(a string) {
	say("")
	say(styleAgent.Render("● agent ") + wrap(a, 8))
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
