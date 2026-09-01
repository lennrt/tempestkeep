package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/config"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// nowConfig is the resolved data source for `tempestkeep now`: a live client (with a
// pre-resolved station) and/or a read-only archive to fall back on.
type nowConfig struct {
	live  *nowLiveSource
	store *store.Store
}

// nowLiveSource resolves and caches the station behind the dashboard's splash
// instead of delaying TUI startup with network I/O.
type nowLiveSource struct {
	client  *api.Client
	mu      sync.Mutex
	station *api.Station
}

func (l *nowLiveSource) resolve(ctx context.Context) (*api.Station, error) {
	l.mu.Lock()
	if l.station != nil {
		station := l.station
		l.mu.Unlock()
		return station, nil
	}
	l.mu.Unlock()

	station, _, err := l.client.FindTempestDevice(ctx)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.station != nil {
		return l.station, nil
	}
	l.station = station
	return station, nil
}

func (l *nowLiveSource) stationName() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.station == nil {
		return ""
	}
	return l.station.Name
}

// load fetches one frame of data. Live is preferred; if the live fetch fails and
// an archive is present, it falls back so the dashboard still shows something.
func (c nowConfig) load(ctx context.Context) (dashboard, error) {
	if c.live != nil {
		station, err := c.live.resolve(ctx)
		if err == nil {
			fetchCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			type observationResult struct {
				obs *api.StationObs
				err error
			}
			type forecastResult struct {
				forecast *api.Forecast
				err      error
			}
			obsCh := make(chan observationResult, 1)
			forecastCh := make(chan forecastResult, 1)
			go func() {
				obs, err := c.live.client.LatestStationObs(fetchCtx, station.StationID)
				obsCh <- observationResult{obs: obs, err: err}
			}()
			go func() {
				forecast, err := c.live.client.BetterForecast(fetchCtx, station.StationID)
				forecastCh <- forecastResult{forecast: forecast, err: err}
			}()
			gotObs := <-obsCh
			if gotObs.err != nil {
				cancel()
				<-forecastCh
				err = gotObs.err
			} else {
				gotForecast := <-forecastCh
				d := buildLiveDashboard(station, gotObs.obs, gotForecast.forecast)
				if gotForecast.err != nil {
					d.note = "live forecast unavailable"
				}
				return d, nil
			}
		}
		if c.store == nil {
			return dashboard{}, err
		}
		// fall through to the archive
	}
	if c.store != nil {
		o, err := c.store.Latest(ctx)
		if err != nil {
			return dashboard{}, err
		}
		if o == nil {
			return dashboard{}, fmt.Errorf("archive has no observations yet; run `tempestkeep collect` first")
		}
		stationName := ""
		if c.live != nil {
			stationName = c.live.stationName()
		}
		d := buildArchiveDashboard(stationName, o)
		if err := fillArchiveRainToday(ctx, c.store, &d, time.Now()); err != nil {
			return dashboard{}, err
		}
		return d, nil
	}
	return dashboard{}, fmt.Errorf("no data source available")
}

func fillArchiveRainToday(ctx context.Context, st *store.Store, d *dashboard, now time.Time) error {
	obsLocal := d.obsTime.Local()
	nowLocal := now.Local()
	if obsLocal.Year() != nowLocal.Year() || obsLocal.YearDay() != nowLocal.YearDay() {
		return nil
	}
	start := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.Local)
	days, err := st.DailySummary(ctx, start.Unix(), now.Unix())
	if err != nil {
		return err
	}
	if len(days) == 0 {
		return nil
	}
	rain := days[len(days)-1].RainIn
	d.rainTodayIn = &rain
	return nil
}

// ---- bubbletea model --------------------------------------------------------

type fetchedMsg struct {
	d   dashboard
	err error
}

type tickMsg time.Time

// splashTickMsg drives the splash-screen spring animation at splashFPS until
// the first frame of data arrives.
type splashTickMsg time.Time

const splashFPS = 30

// splashDrop is where the splash starts, in rows above its resting place; the
// spring settles it to zero.
const splashDrop = 8.0

type nowModel struct {
	cfg       nowConfig
	interval  time.Duration
	d         dashboard
	haveData  bool
	loading   bool
	err       error
	lastFetch time.Time
	width     int
	height    int

	spin      spinner.Model
	spring    harmonica.Spring
	splashPos float64 // rows above resting position (animates toward 0)
	splashVel float64
}

func newNowModel(cfg nowConfig, interval time.Duration) nowModel {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("#5B9BD5"))))
	return nowModel{
		cfg: cfg, interval: interval, loading: true,
		spin:      sp,
		spring:    harmonica.NewSpring(harmonica.FPS(splashFPS), 5.0, 0.8),
		splashPos: splashDrop,
	}
}

func (m nowModel) Init() tea.Cmd {
	return tea.Batch(m.fetch(), tick(m.interval), m.spin.Tick, splashTick())
}

func splashTick() tea.Cmd {
	return tea.Tick(time.Second/splashFPS, func(t time.Time) tea.Msg { return splashTickMsg(t) })
}

func (m nowModel) fetch() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		d, err := cfg.load(ctx)
		return fetchedMsg{d: d, err: err}
	}
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m nowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			if m.loading {
				return m, nil
			}
			m.loading = true
			return m, m.fetch()
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case fetchedMsg:
		m.loading = false
		m.lastFetch = time.Now()
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.d, m.haveData, m.err = msg.d, true, nil
		}
	case tickMsg:
		if m.loading {
			return m, tick(m.interval)
		}
		m.loading = true
		return m, tea.Batch(m.fetch(), tick(m.interval))
	case splashTickMsg:
		if !m.haveData { // keep the spring stepping only while the splash shows
			m.splashPos, m.splashVel = m.spring.Update(m.splashPos, m.splashVel, 0)
			return m, splashTick()
		}
	case spinner.TickMsg:
		if !m.haveData {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m nowModel) View() string {
	var body string
	switch {
	case !m.haveData && m.err != nil:
		body = errorCard(m.err)
	case !m.haveData:
		// Splash: the wordmark settles in on a spring while the spinner
		// waits for the first frame of data.
		drop := int(m.splashPos + 0.5)
		// Archive-only mode never touches the network, so don't claim to be
		// contacting the station (mirrors explore's "reading your archive…").
		wait := " contacting your station…"
		if m.cfg.live == nil {
			wait = " reading your archive…"
		}
		body = lipgloss.NewStyle().PaddingTop(drop).Render(
			lipgloss.JoinVertical(lipgloss.Center,
				splashArt(),
				"",
				m.spin.View()+faint().Render(wait),
			))
	case m.width > 0 && m.width < nowMinWidth:
		body = narrowNotice(m.width, nowMinWidth)
	default:
		body = renderDashboard(m.d, time.Now(), m.footer())
	}
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
	}
	return body
}

func (m nowModel) footer() string {
	hint := "r refresh · q quit"
	if m.loading {
		return hint + " · updating…"
	}
	if m.err != nil { // transient error after we already have data
		return hint + " · last update failed, showing cached"
	}
	if !m.lastFetch.IsZero() {
		return hint + " · updated " + m.lastFetch.Format("15:04:05")
	}
	return hint
}

func errorCard(err error) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#E63946")).
		Padding(1, 3).
		Render(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E63946")).Render("Couldn't load Tempest data") +
			"\n" + faint().Render(err.Error()))
}

// ---- command ----------------------------------------------------------------

// cmdNow implements `tempestkeep now`: a live, auto-refreshing dashboard, or a single
// rendered frame with --once (pipe-friendly, like wttr.in).
func cmdNow(args []string) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fs := flag.NewFlagSet("now", flag.ContinueOnError)
	describe(fs, "tempestkeep now: current conditions as a live terminal dashboard. Use --once\nfor a single frame, or --format json for scriptable output.",
		"tempestkeep now",
		"tempestkeep now --once",
		"tempestkeep now --format json | jq .temp_f")
	db := fs.String("db", "", "path to the tempest.sqlite archive (or env TEMPEST_DB)")
	once := fs.Bool("once", false, "render one frame to stdout and exit (no interactive UI)")
	format := fs.String("format", "text", "output format: text or json (json implies one frame, like --once)")
	intervalSec := fs.Int("interval", 60, "seconds between refreshes in interactive mode (minimum 5)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	*format = strings.ToLower(*format)
	if *format != "text" && *format != "json" {
		return usagef("--format must be text or json")
	}
	if *intervalSec < 5 {
		return usagef("--interval must be at least 5 seconds")
	}
	if err := config.LoadDotenv(ctx, ".env"); err != nil {
		return err
	}

	// A piped or redirected stdout gets one clean frame, not an alt-screen TUI
	// whose control sequences would land in the file.
	oneShot := *once || *format == "json" || !isTTY(os.Stdout)

	cfg, err := resolveNowConfig(ctx, *db)
	if err != nil {
		return err
	}
	if cfg.store != nil {
		defer func() { err = errors.Join(err, cfg.store.Close()) }()
	}

	// json and --once are both one-shot: fetch a single frame and exit, so the
	// command composes in pipes instead of taking over the terminal.
	if oneShot {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		d, err := cfg.load(ctx)
		if err != nil {
			return err
		}
		if *format == "json" {
			return writeNowJSON(os.Stdout, d)
		}
		_, err = fmt.Fprintln(os.Stdout, renderDashboard(d, time.Now(), ""))
		return err
	}

	interval := time.Duration(*intervalSec) * time.Second
	p := tea.NewProgram(newNowModel(cfg, interval), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err = p.Run()
	return err
}

// nowJSON is the machine-readable shape of `tempestkeep now --format json`: the same
// current conditions the card shows, in display units, with json-stable keys
// that mirror the MCP current_conditions tool so scripts see one schema.
type nowJSON struct {
	Source              string   `json:"source"` // "live" or "archive"
	Station             string   `json:"station,omitempty"`
	Time                string   `json:"time"`
	Conditions          string   `json:"conditions,omitempty"`
	TempF               *float64 `json:"temp_f,omitempty"`
	FeelsLikeF          *float64 `json:"feels_like_f,omitempty"`
	DewPointF           *float64 `json:"dew_point_f,omitempty"`
	HumidityPct         *float64 `json:"humidity_pct,omitempty"`
	PressureInHg        *float64 `json:"pressure_inhg,omitempty"`
	WindMph             *float64 `json:"wind_mph,omitempty"`
	GustMph             *float64 `json:"gust_mph,omitempty"`
	WindDir             string   `json:"wind_dir,omitempty"`
	WindDirDeg          *float64 `json:"wind_dir_deg,omitempty"`
	UV                  *float64 `json:"uv,omitempty"`
	SolarWm2            *float64 `json:"solar_wm2,omitempty"`
	RainTodayIn         *float64 `json:"rain_today_in,omitempty"`
	LightningStrikes1hr *int     `json:"lightning_strikes_1hr,omitempty"`
	Note                string   `json:"note,omitempty"`
}

func writeNowJSON(w io.Writer, d dashboard) error {
	j := nowJSON{
		Source: d.source, Station: d.station, Time: d.obsTime.Format(time.RFC3339),
		Conditions: d.conditions, TempF: d.tempF, FeelsLikeF: d.feelsF,
		DewPointF: d.dewF, HumidityPct: d.humidityPct, PressureInHg: d.pressureInHg,
		WindMph: d.windMph, GustMph: d.gustMph, WindDirDeg: d.windDirDeg,
		UV: d.uv, SolarWm2: d.solarWm2, RainTodayIn: d.rainTodayIn,
		LightningStrikes1hr: d.lightning1h, Note: d.note,
	}
	if d.windDirDeg != nil {
		j.WindDir = model.Compass(*d.windDirDeg)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(j)
}

// resolveNowConfig wires up the live client and/or archive from token/db without
// making a network call. Station discovery happens behind the dashboard splash.
func resolveNowConfig(ctx context.Context, dbFlag string) (nowConfig, error) {
	token := os.Getenv("TEMPEST_TOKEN")
	dbPath, err := config.ResolveDB(ctx, dbFlag)
	if err != nil {
		return nowConfig{}, err
	}

	var cfg nowConfig
	if dbPath != "" {
		s, err := store.Open(ctx, dbPath)
		if err != nil {
			return cfg, fmt.Errorf("open configured archive: %w", err)
		}
		cfg.store = s
	}
	if token != "" {
		client, err := newAPIClient(token)
		if err != nil {
			return cfg, err
		}
		cfg.live = &nowLiveSource{client: client}
	}
	if cfg.live == nil && cfg.store == nil {
		return cfg, fmt.Errorf("no data source: run `tempestkeep setup`, set TEMPEST_TOKEN, or set --db/TEMPEST_DB")
	}
	return cfg, nil
}
