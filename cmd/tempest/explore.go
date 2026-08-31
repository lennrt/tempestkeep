package main

// `tempest explore` is the historical explorer TUI. Where
// `tempest now` is one live card, explore is a scrubbable window over the whole
// archive: day / week / month / year views plus an all-time records board, all
// answered by the same read-only store queries the MCP history tools use. No
// token required: this is the archive you own, offline.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// exploreView enumerates the explorer's five views, in tab order.
type exploreView int

const (
	viewDay exploreView = iota
	viewWeek
	viewMonth
	viewYear
	viewRecords
)

var viewNames = [...]string{"day", "week", "month", "year", "records"}

// exploreWidth is the inner content width of the explorer card. Wide enough
// for a 48-column day chart with a value gutter and a 53-week year ribbon.
const exploreWidth = 60

// exploreMinWidth is the terminal width the card needs: exploreWidth plus the
// rounded border (2) and horizontal padding (3 each side). Below it, the card
// border wraps and every row misaligns, so degrade to a plain notice instead.
const exploreMinWidth = exploreWidth + 2 + 6

// exploreData is everything one view render needs, fetched in a single
// tea.Cmd so the UI never blocks on SQLite.
type exploreData struct {
	label   string // human period label, e.g. "July 2026"
	day     dayData
	week    []store.DayStat
	month   []store.DayStat
	year    yearData
	records recordsData
}

type dayData struct {
	points []store.SeriesPoint // half-hour buckets across the day
	stat   *store.DayStat      // the day's aggregate, nil when empty
}

type yearData struct {
	days   []store.DayStat    // one per observed day, for the ribbon
	months []store.PeriodStat // monthly climatology strip
}

type recordsData struct {
	records store.Records
	thisDay []store.YearDay // today's calendar day across all years
}

type exploreDataMsg struct {
	gen  int // generation guard: stale fetches are dropped
	data exploreData
	err  error
}

type exploreModel struct {
	st  *store.Store
	cov store.Coverage

	view   exploreView
	offset int // periods back from the present (0 = current period)
	metric int // index into heatMetrics, cycled with tab

	gen     int
	data    exploreData
	haveOne bool // first fetch landed; before that, show the splash
	loading bool
	err     error

	width, height int
	spin          spinner.Model
}

func newExploreModel(st *store.Store, cov store.Coverage) exploreModel {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#5B9BD5"))))
	return exploreModel{st: st, cov: cov, loading: true, spin: sp}
}

func (m exploreModel) Init() tea.Cmd {
	return tea.Batch(m.fetch(), m.spin.Tick)
}

// fetch loads the current (view, offset) in the background. The generation
// counter makes rapid scrubbing safe: only the newest request's reply lands.
func (m exploreModel) fetch() tea.Cmd {
	st, view, offset, gen := m.st, m.view, m.offset, m.gen
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		data, err := loadExplore(ctx, st, view, offset)
		return exploreDataMsg{gen: gen, data: data, err: err}
	}
}

func (m exploreModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case exploreDataMsg:
		if msg.gen != m.gen {
			return m, nil // a newer fetch is already in flight
		}
		m.loading = false
		m.haveOne = true
		m.data, m.err = msg.data, msg.err
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m exploreModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit

	case "left", "h":
		return m.scrub(+1)
	case "right", "l":
		return m.scrub(-1)
	case "g", "home":
		if m.offset == 0 {
			return m, nil
		}
		m.offset = 0
		return m.refetch()

	case "tab":
		m.metric = (m.metric + 1) % len(heatMetrics)
		return m, nil // pure re-render; the data already holds every metric

	case "d", "1":
		return m.switchView(viewDay)
	case "w", "2":
		return m.switchView(viewWeek)
	case "m", "3":
		return m.switchView(viewMonth)
	case "y", "4":
		return m.switchView(viewYear)
	case "r", "5":
		return m.switchView(viewRecords)
	}
	return m, nil
}

// scrub moves delta periods back (+) or forward (−), clamped to the archive's
// coverage on one side and the present on the other.
func (m exploreModel) scrub(delta int) (tea.Model, tea.Cmd) {
	if m.view == viewRecords {
		return m, nil // records are all-time; nothing to scrub
	}
	next := m.offset + delta
	if next < 0 || next > m.maxOffset() {
		return m, nil
	}
	m.offset = next
	return m.refetch()
}

func (m exploreModel) switchView(v exploreView) (tea.Model, tea.Cmd) {
	if v == m.view {
		return m, nil
	}
	m.view = v
	m.offset = 0
	return m.refetch()
}

func (m exploreModel) refetch() (tea.Model, tea.Cmd) {
	m.gen++
	m.loading = true
	return m, m.fetch()
}

// maxOffset is how many whole periods back the archive's oldest observation
// allows the current view to scrub.
func (m exploreModel) maxOffset() int {
	if !m.cov.MinEpoch.Valid {
		return 0
	}
	oldest := time.Unix(m.cov.MinEpoch.Int64, 0).Local()
	now := time.Now()
	switch m.view {
	case viewDay:
		return daysBetween(midnightOf(oldest), midnightOf(now))
	case viewWeek:
		return daysBetween(mondayOf(oldest), mondayOf(now)) / 7
	case viewMonth:
		return (now.Year()-oldest.Year())*12 + int(now.Month()) - int(oldest.Month())
	case viewYear:
		return now.Year() - oldest.Year()
	case viewRecords:
		return 0
	default:
		return 0
	}
}

// ---- period math --------------------------------------------------------------

func midnightOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// mondayOf returns local midnight of the Monday on or before t (weeks render
// Monday-first, like the month calendar).
func mondayOf(t time.Time) time.Time {
	back := (int(t.Weekday()) + 6) % 7
	return midnightOf(t).AddDate(0, 0, -back)
}

// daysBetween counts calendar days from a to b, both local midnights with a
// not after b. Truncating Sub().Hours()/24 undercounts by one across every
// spring-forward transition (a 23-hour day); rounding absorbs the DST hour.
func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours()/24 + 0.5)
}

// periodRange resolves (view, offset) to the local [start, end) window it
// covers and a human label for the header.
func periodRange(view exploreView, offset int, now time.Time) (start, end time.Time, label string) {
	switch view {
	case viewDay:
		start = midnightOf(now).AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 1), start.Format("Mon, Jan 2 2006")
	case viewWeek:
		start = mondayOf(now).AddDate(0, 0, -7*offset)
		return start, start.AddDate(0, 0, 7), "week of " + start.Format("Jan 2, 2006")
	case viewMonth:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -offset, 0)
		return start, start.AddDate(0, 1, 0), start.Format("January 2006")
	case viewYear:
		start = time.Date(now.Year()-offset, 1, 1, 0, 0, 0, 0, time.Local)
		return start, start.AddDate(1, 0, 0), start.Format("2006")
	case viewRecords: // records span the whole archive
		return time.Time{}, now, "all time"
	default:
		return time.Time{}, time.Time{}, "invalid view"
	}
}

// dayBucketSeconds is the day view's series resolution: half-hour buckets, 48
// columns across a day, chart-sized without smoothing away the shape.
const dayBucketSeconds = 1800

// loadExplore runs the queries one (view, offset) needs. It's the only place
// the explorer touches the store, and everything it runs is read-only.
func loadExplore(ctx context.Context, st *store.Store, view exploreView, offset int) (exploreData, error) {
	start, end, label := periodRange(view, offset, time.Now())
	d := exploreData{label: label}

	switch view {
	case viewDay:
		points, err := st.Series(ctx, start.Unix(), end.Unix()-1, dayBucketSeconds)
		if err != nil {
			return d, err
		}
		days, err := st.DailySummary(ctx, start.Unix(), end.Unix()-1)
		if err != nil {
			return d, err
		}
		d.day.points = points
		if len(days) > 0 {
			d.day.stat = &days[0]
		}

	case viewWeek:
		days, err := st.DailySummary(ctx, start.Unix(), end.Unix()-1)
		if err != nil {
			return d, err
		}
		d.week = days

	case viewMonth:
		days, err := st.DailySummary(ctx, start.Unix(), end.Unix()-1)
		if err != nil {
			return d, err
		}
		d.month = days

	case viewYear:
		days, err := st.DailySummary(ctx, start.Unix(), end.Unix()-1)
		if err != nil {
			return d, err
		}
		months, err := st.PeriodSummary(ctx, store.PeriodMonth, start.Unix(), end.Unix()-1)
		if err != nil {
			return d, err
		}
		d.year = yearData{days: days, months: months}

	case viewRecords:
		rec, err := st.Records(ctx)
		if err != nil {
			return d, err
		}
		now := time.Now()
		thisDay, err := st.ThisDay(ctx, int(now.Month()), now.Day())
		if err != nil {
			return d, err
		}
		d.records = recordsData{records: rec, thisDay: thisDay}
	}
	return d, nil
}

// ---- view ----------------------------------------------------------------------

func (m exploreModel) View() string {
	var body string
	switch {
	case !m.haveOne && m.err != nil:
		body = errorCard(m.err)
	case !m.haveOne:
		body = lipgloss.JoinVertical(lipgloss.Center,
			splashArt(), "",
			m.spin.View()+faint().Render(" reading your archive…"))
	case m.width > 0 && m.width < exploreMinWidth:
		body = narrowNotice(m.width, exploreMinWidth)
	default:
		body = m.renderCard()
	}
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
	}
	return body
}

// renderCard draws the whole explorer card: view tabs, period header, the
// active view's body, and the key help footer.
func (m exploreModel) renderCard() string {
	accent := lipgloss.Color("#5B9BD5")

	sections := []string{
		m.tabsLine(),
		divider2(),
		m.headerLine(),
		"",
		m.bodyFor(),
		divider2(),
		faint().Render(m.footerHelp()),
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 3).
		Render(lipgloss.JoinVertical(lipgloss.Left, sections...))
}

// tabsLine renders the view switcher, active view highlighted.
func (m exploreModel) tabsLine() string {
	active := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD23F"))
	var tabs strings.Builder
	for v, name := range viewNames {
		if v > 0 {
			tabs.WriteString(faint().Render("  ·  "))
		}
		if exploreView(v) == m.view {
			tabs.WriteString(active.Render(name))
		} else {
			tabs.WriteString(faint().Render(name))
		}
	}
	// No emoji here: lipgloss measures ⛅ at 2 cells but many terminals draw
	// it at 1, which pulls this line's right border out of column.
	title := lipgloss.NewStyle().Bold(true).Render("explore")
	return spread(title, tabs.String(), exploreWidth)
}

// headerLine shows the period being viewed and the archive's total coverage.
func (m exploreModel) headerLine() string {
	label := lipgloss.NewStyle().Bold(true).Render(m.data.label)
	if m.loading {
		label += " " + m.spin.View()
	}
	right := ""
	if m.cov.MinEpoch.Valid && m.cov.MaxEpoch.Valid {
		right = faint().Render(fmt.Sprintf("archive %s → %s",
			time.Unix(m.cov.MinEpoch.Int64, 0).Local().Format("Jan 2006"),
			time.Unix(m.cov.MaxEpoch.Int64, 0).Local().Format("Jan 2006")))
	}
	return spread(label, right, exploreWidth)
}

func (m exploreModel) bodyFor() string {
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#E63946")).
			Render("couldn't load this view: " + m.err.Error())
	}
	switch m.view {
	case viewDay:
		return renderDayView(m.data.day)
	case viewWeek:
		return renderWeekView(m.data.week, m.offset)
	case viewMonth:
		return renderMonthView(m.data.month, m.offset, heatMetrics[m.metric])
	case viewYear:
		return renderYearView(m.data.year, m.offset, heatMetrics[m.metric])
	case viewRecords:
		return renderRecordsView(m.data.records)
	default:
		return "invalid view"
	}
}

func (m exploreModel) footerHelp() string {
	help := "←/→ scrub · d w m y r views · g latest · q quit"
	if m.view == viewMonth || m.view == viewYear {
		help = "←/→ scrub · tab metric · d w m y r views · g latest · q quit"
	}
	if m.view == viewRecords {
		help = "d w m y r views · q quit"
	}
	return help
}

// divider2 is the explorer-width rule (the `now` card has its own width).
func divider2() string { return faint().Render(dividerLine(exploreWidth)) }

// ---- command --------------------------------------------------------------------

// cmdExplore implements `tempest explore`: an interactive, scrubbable history
// browser over the local archive. Archive-only by design, so no token is needed.
func cmdExplore(args []string) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fs := flag.NewFlagSet("explore", flag.ContinueOnError)
	describe(fs, "tempest explore: browse the archive interactively: day, week, month, year,\nand all-time-records views; scrub back through history with ←/→.",
		"tempest explore",
		"tempest explore --db ~/weather/tempest.sqlite")
	db := fs.String("db", "", "path to the tempest.sqlite archive (or env TEMPEST_DB)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if !isTTY(os.Stdout) {
		// A full-screen TUI would write control sequences into a pipe.
		return errors.New("explore is interactive and needs a terminal; for scriptable output use `tempest stats` or `tempest export`")
	}
	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}

	dbPath, err := config.ResolveDB(ctx, *db)
	if err != nil {
		return err
	}
	if dbPath == "" {
		return fmt.Errorf("no archive configured: set --db/TEMPEST_DB, or run `tempest setup`")
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Close()) }()

	cov, err := st.Coverage(ctx)
	if err != nil {
		return err
	}
	if cov.Count == 0 {
		return fmt.Errorf("archive is empty; run `tempest collect` first")
	}

	p := tea.NewProgram(newExploreModel(st, cov), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err = p.Run()
	return err
}
