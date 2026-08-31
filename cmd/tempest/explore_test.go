package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// ---- period math ----------------------------------------------------------------

func TestMondayOf(t *testing.T) {
	// 2026-07-13 is a Monday; every day of that week maps back to it.
	monday := time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local)
	for i := range 7 {
		day := monday.AddDate(0, 0, i).Add(15 * time.Hour)
		if got := mondayOf(day); !got.Equal(monday) {
			t.Errorf("mondayOf(%s) = %s, want %s", day.Format("Mon"), got, monday)
		}
	}
}

func TestDaysBetweenAcrossDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	mid := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, ny)
	}
	// The 2026 US spring-forward day (Mar 8) is 23 hours; truncating
	// Hours()/24 would call it 0 days.
	if got := daysBetween(mid(2026, time.March, 8), mid(2026, time.March, 9)); got != 1 {
		t.Errorf("spring-forward day = %d days, want 1", got)
	}
	// A winter-to-summer span crosses the transition once.
	if got := daysBetween(mid(2026, time.January, 5), mid(2026, time.July, 13)); got != 189 {
		t.Errorf("Jan 5 -> Jul 13 = %d days, want 189", got)
	}
	// The fall-back day (Nov 1) is 25 hours; rounding must not call it 2.
	if got := daysBetween(mid(2026, time.November, 1), mid(2026, time.November, 2)); got != 1 {
		t.Errorf("fall-back day = %d days, want 1", got)
	}
}

func TestPeriodRange(t *testing.T) {
	now := time.Date(2026, 7, 13, 14, 30, 0, 0, time.Local)

	start, end, label := periodRange(viewDay, 1, now)
	if want := time.Date(2026, 7, 12, 0, 0, 0, 0, time.Local); !start.Equal(want) {
		t.Errorf("day-1 start = %s, want %s", start, want)
	}
	if got := end.Sub(start); got != 24*time.Hour {
		t.Errorf("day span = %s", got)
	}
	if label == "" {
		t.Error("day label empty")
	}

	start, end, _ = periodRange(viewMonth, 1, now)
	if want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local); !start.Equal(want) {
		t.Errorf("month-1 start = %s, want %s", start, want)
	}
	if want := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local); !end.Equal(want) {
		t.Errorf("month-1 end = %s, want %s", end, want)
	}

	start, _, label = periodRange(viewYear, 2, now)
	if start.Year() != 2024 || label != "2024" {
		t.Errorf("year-2 = %s / %q", start, label)
	}

	_, _, label = periodRange(viewRecords, 0, now)
	if label != "all time" {
		t.Errorf("records label = %q", label)
	}
}

// ---- chart primitives -------------------------------------------------------------

func TestColumnChartShape(t *testing.T) {
	vals := []*float64{new(float64(0)), new(float64(50)), new(float64(100)), nil}
	rows := columnChart(vals, 3, func(int, float64) lipgloss.Color { return lipgloss.Color("1") })
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	for r, row := range rows {
		if got := lipgloss.Width(row); got != len(vals) {
			t.Errorf("row %d width = %d, want %d", r, got, len(vals))
		}
	}
	// The max value must fill the top row's column; the min must not.
	if !strings.Contains(rows[0], "█") {
		t.Errorf("top row has no full block: %q", rows[0])
	}
}

func TestSparklineGaps(t *testing.T) {
	got := sparkline([]*float64{new(float64(1)), nil, new(float64(3))}, func(int, float64) lipgloss.Color { return lipgloss.Color("2") })
	if lipgloss.Width(got) != 3 {
		t.Errorf("width = %d, want 3", lipgloss.Width(got))
	}
	if !strings.Contains(got, gapDot) {
		t.Errorf("nil value should render the gap dot: %q", got)
	}
}

func TestRangeBandWidth(t *testing.T) {
	for _, w := range []int{2, 10, 20} {
		got := rangeBand(60, 90, 50, 100, w, tempColor)
		if lipgloss.Width(got) != w {
			t.Errorf("band width = %d, want %d", lipgloss.Width(got), w)
		}
	}
	// Degenerate scale must not divide by zero or overflow the width.
	if got := rangeBand(70, 70, 70, 70, 10, tempColor); lipgloss.Width(got) != 10 {
		t.Errorf("flat-scale band width = %d, want 10", lipgloss.Width(got))
	}
}

// ---- view smoke test over a real archive ------------------------------------------

// seedArchive writes ~60 days of synthetic 20-minute observations ending now,
// so every view (including a previous month) has data to draw.
func seedArchive(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "explore.sqlite")
	w, err := store.OpenWriter(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	ctx := context.Background()

	now := time.Now().Unix()
	start := now - 60*86400
	var obs []model.DeviceObs
	for e := start - start%1200; e <= now; e += 1200 {
		hour := float64(time.Unix(e, 0).Local().Hour())
		temp := 18 + 8*hour/24 // a crude diurnal slope is plenty for rendering
		wind := 1 + hour/6
		gust := wind * 1.7
		rain := 0.0
		if time.Unix(e, 0).Local().Day()%9 == 0 && hour == 18 {
			rain = 0.4
		}
		obs = append(obs, model.DeviceObs{
			Epoch: e, AirTempC: new(temp), Humidity: new(float64(55)),
			WindAvg: new(wind), WindGust: new(gust), WindDir: new(float64(250)),
			PressureMb: new(float64(975)), UV: new(float64(5)), SolarWm2: new(float64(400)), RainMm: new(rain),
		})
	}
	if _, err := w.InsertObs(ctx, 4242, obs); err != nil {
		t.Fatalf("InsertObs: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	closeOnCleanup(t, st)
	return st
}

// TestExploreViewsRender drives loadExplore + every renderer over a seeded
// archive: each view must produce output that fits the card width.
func TestExploreViewsRender(t *testing.T) {
	st := seedArchive(t)
	ctx := context.Background()

	cases := []struct {
		view   exploreView
		offset int
	}{
		{viewDay, 0}, {viewDay, 1},
		{viewWeek, 0}, {viewWeek, 1},
		{viewMonth, 0}, {viewMonth, 1},
		{viewYear, 0},
		{viewRecords, 0},
	}
	for _, tc := range cases {
		data, err := loadExplore(ctx, st, tc.view, tc.offset)
		if err != nil {
			t.Fatalf("loadExplore(%s, %d): %v", viewNames[tc.view], tc.offset, err)
		}
		var body string
		switch tc.view {
		case viewDay:
			body = renderDayView(data.day)
		case viewWeek:
			body = renderWeekView(data.week, tc.offset)
		case viewMonth:
			body = renderMonthView(data.month, tc.offset, heatMetrics[0])
		case viewYear:
			body = renderYearView(data.year, tc.offset, heatMetrics[2])
		case viewRecords:
			body = renderRecordsView(data.records)
		}
		if strings.TrimSpace(body) == "" {
			t.Errorf("%s/%d rendered empty", viewNames[tc.view], tc.offset)
			continue
		}
		for line := range strings.SplitSeq(body, "\n") {
			if w := lipgloss.Width(line); w > exploreWidth {
				t.Errorf("%s/%d line overflows card: %d > %d: %q",
					viewNames[tc.view], tc.offset, w, exploreWidth, line)
			}
		}
	}
}

func TestRecordsViewPadsLongestLabel(t *testing.T) {
	pressure := 28.69
	body := renderRecordsView(recordsData{records: store.Records{LowestPressureInHg: &pressure}})
	if !strings.Contains(body, "low pressure ") {
		t.Fatalf("longest record label has no separator before its value:\n%s", body)
	}
}

// TestDayViewLowLabelUsesTrueExtremes guards the day-chart gutter labels. The
// plotted line is half-hour averages, but the hi/lo labels must come from the
// day's true per-observation extremes, not the min/max of those averages (which
// reads too warm because a brief dip survives inside a bucket mean). Regression
// for a chart that showed "lo 61" when the real low was several degrees colder.
func TestDayViewLowLabelUsesTrueExtremes(t *testing.T) {
	base := midnightOf(time.Now())
	var pts []store.SeriesPoint
	for i := range 7 {
		avg := 64.0 + float64(i) // 64..70, all warmer than the true low
		pts = append(pts, store.SeriesPoint{
			Epoch:    base.Add(time.Duration(i) * dayBucketSeconds * time.Second).Unix(),
			TempAvgF: &avg,
			Obs:      1,
		})
	}
	lo, hi, avg := 58.0, 79.0, 67.0 // true extremes sit outside the average band
	d := dayData{
		points: pts,
		stat:   &store.DayStat{TempMinF: &lo, TempMaxF: &hi, TempAvgF: &avg, Obs: int64(len(pts))},
	}
	body := renderDayView(d)
	if !strings.Contains(body, "58°") || !strings.Contains(body, "79°") {
		t.Errorf("day chart should label the gutter with the true extremes 58°/79°:\n%s", body)
	}
	// The min/max of the plotted averages (64°/70°) only ever surface as gutter
	// labels; their presence would mean the gutter still reads the averaged series.
	if strings.Contains(body, "64°") || strings.Contains(body, "70°") {
		t.Errorf("day chart gutter still shows the average band (64°/70°), not the true extremes:\n%s", body)
	}
}

// TestExploreMetricsCycle re-renders the month view under every metric: the
// tab key must never hit a metric that panics on partial data.
func TestExploreMetricsCycle(t *testing.T) {
	st := seedArchive(t)
	data, err := loadExplore(context.Background(), st, viewMonth, 0)
	if err != nil {
		t.Fatalf("loadExplore: %v", err)
	}
	for _, m := range heatMetrics {
		if body := renderMonthView(data.month, 0, m); strings.TrimSpace(body) == "" {
			t.Errorf("metric %q rendered empty", m.name)
		}
	}
}
