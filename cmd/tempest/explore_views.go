package main

// The five explorer views. Each renderer takes the already-fetched data plus
// the scrub offset (to recompute its own period bounds) and returns a block of
// styled lines exploreWidth wide. Everything is drawn with the primitives in
// charts.go: no plotting library, no NerdFont requirement.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

// ---- heat metrics (month/year coloring, cycled with tab) ----------------------

// heatMetric is one way to tint a calendar cell: which value of a day it
// reads, and how that value maps to a color. Temperatures use the absolute
// cold→hot ramp shared with `tempest now`; rain and gust normalize to the
// period being viewed, so "wettest day this month" always saturates.
type heatMetric struct {
	name   string
	value  func(store.DayStat) *float64
	color  func(v, lo, hi float64) lipgloss.Color
	format string // for the legend endpoints
}

var heatMetrics = []heatMetric{
	{"high °F", func(d store.DayStat) *float64 { return d.TempMaxF },
		func(v, _, _ float64) lipgloss.Color { return tempColor(v) }, "%.0f°"},
	{"low °F", func(d store.DayStat) *float64 { return d.TempMinF },
		func(v, _, _ float64) lipgloss.Color { return tempColor(v) }, "%.0f°"},
	{"rain", func(d store.DayStat) *float64 { v := d.RainIn; return &v },
		rainColor, "%.2f in"},
	{"gust", func(d store.DayStat) *float64 { return d.PeakGustMph },
		gustColor, "%.0f mph"},
}

// ---- day view ------------------------------------------------------------------

// renderDayView draws one day: a 4-row temperature chart over half-hour
// buckets, labeled sparklines for rain, wind, and solar, and the day's
// aggregate stats.
func renderDayView(d dayData) string {
	if len(d.points) == 0 {
		return emptyView("no observations on this day")
	}

	// Column index = position of the bucket within the local day. DST days
	// hold 46 or 50 half-hour buckets, so size from the actual day instead of
	// a 48-column constant that would drop the fall-back day's last hour.
	midnight := midnightOf(time.Unix(d.points[0].Epoch, 0).Local())
	cols := int(midnight.AddDate(0, 0, 1).Sub(midnight) / (dayBucketSeconds * time.Second))
	temps := make([]*float64, cols)
	rain := make([]*float64, cols)
	wind := make([]*float64, cols)
	solar := make([]*float64, cols)
	for _, p := range d.points {
		i := int((p.Epoch - midnight.Unix()) / dayBucketSeconds)
		if i < 0 || i >= cols {
			continue
		}
		temps[i] = p.TempAvgF
		if p.RainIn > 0 {
			v := p.RainIn
			rain[i] = &v
		} else if p.Obs > 0 {
			zero := 0.0
			rain[i] = &zero
		}
		wind[i] = p.WindMph
		solar[i] = p.SolarAvgWm2
	}

	const gutter = 6
	pad := strings.Repeat(" ", gutter)
	// The chart line plots half-hour averages, so its own min/max understate the
	// day: a brief dip lives on inside a bucket's mean. Label the gutter from the
	// day's true per-observation extremes instead, so the chart's hi/lo match the
	// stat row below it (which reads the same TempMinF/TempMaxF).
	lo, hi, ok := minMaxVals(temps)
	if ok && d.stat != nil {
		if d.stat.TempMinF != nil {
			lo = *d.stat.TempMinF
		}
		if d.stat.TempMaxF != nil {
			hi = *d.stat.TempMaxF
		}
	}

	var lines []string
	chart := columnChart(temps, 4, func(_ int, v float64) lipgloss.Color { return tempColor(v) })
	for r, row := range chart {
		label := pad
		if ok && r == 0 {
			label = lipgloss.NewStyle().Foreground(tempColor(hi)).Render(fmt.Sprintf("%4.0f° ", hi))
		}
		if ok && r == len(chart)-1 {
			label = lipgloss.NewStyle().Foreground(tempColor(lo)).Render(fmt.Sprintf("%4.0f° ", lo))
		}
		lines = append(lines, label+row)
	}
	lines = append(lines, pad+hourAxis(midnight, cols), "")

	if s := d.statLine(); s != "" {
		lines = append(lines, s, "")
	}
	label := func(s string) string { return faint().Render(fmt.Sprintf("%-*s", gutter, s)) }
	lines = append(lines,
		label("rain")+sparkline(rain, func(_ int, v float64) lipgloss.Color {
			_, rhi, _ := minMaxVals(rain)
			return rainColor(v, 0, rhi)
		}),
		label("wind")+sparkline(wind, func(_ int, v float64) lipgloss.Color {
			_, whi, _ := minMaxVals(wind)
			return gustColor(v, 0, whi)
		}),
		label("solar")+sparkline(solar, func(_ int, v float64) lipgloss.Color { return solarColor(v) }),
	)
	return strings.Join(lines, "\n")
}

// statLine renders a day's aggregate as one readings row ("" when no day).
func (d dayData) statLine() string {
	if d.stat == nil {
		return ""
	}
	s := d.stat
	return statRow([]string{
		valF(s.TempMaxF, "hi %.0f°"),
		valF(s.TempMinF, "lo %.0f°"),
		valF(s.TempAvgF, "avg %.0f°"),
		fmt.Sprintf("rain %.2f in", s.RainIn),
		valF(s.PeakGustMph, "gust %.0f mph"),
	})
}

// hourAxis labels the day chart's columns at six-hour marks, placing each
// label at the bucket its local hour actually occupies, so labels stay under
// the right columns on DST days too.
func hourAxis(midnight time.Time, cols int) string {
	axis := []rune(strings.Repeat(" ", cols))
	for _, h := range []int{0, 6, 12, 18} {
		mark := time.Date(midnight.Year(), midnight.Month(), midnight.Day(), h, 0, 0, 0, time.Local)
		i := int(mark.Sub(midnight) / (dayBucketSeconds * time.Second))
		for j, r := range fmt.Sprintf("%02d:00", h) {
			if k := i + j; k >= 0 && k < cols {
				axis[k] = r
			}
		}
	}
	return faint().Render(string(axis))
}

// ---- week view -----------------------------------------------------------------

// renderWeekView draws Monday→Sunday, one line per day: the day's temperature
// span as a colored band on the week's shared scale, plus rain and gust.
func renderWeekView(days []store.DayStat, offset int) string {
	start, _, _ := periodRange(viewWeek, offset, time.Now())
	byDay := map[string]store.DayStat{}
	var all []*float64
	for _, d := range days {
		byDay[d.Day] = d
		all = append(all, d.TempMinF, d.TempMaxF)
	}
	scaleLo, scaleHi, ok := minMaxVals(all)
	if !ok {
		return emptyView("no observations this week")
	}

	const bandW = 20
	var lines []string
	for i := range 7 {
		day := start.AddDate(0, 0, i)
		label := fmt.Sprintf("%-7s", day.Format("Mon 2"))
		d, have := byDay[day.Format("2006-01-02")]
		if !have || (d.TempMinF == nil && d.TempMaxF == nil) {
			lines = append(lines, label+faint().Render("· no data"))
			continue
		}
		lo, hi := orFallback(d.TempMinF, d.TempMaxF), orFallback(d.TempMaxF, d.TempMinF)
		// The high is padded to a fixed width (every value carries exactly one
		// two-byte °, so byte padding is uniform) or differing digit counts
		// would shift the rain and gust columns per row.
		line := label +
			lipgloss.NewStyle().Foreground(tempColor(lo)).Render(fmt.Sprintf("%4.0f° ", lo)) +
			rangeBand(lo, hi, scaleLo, scaleHi, bandW, tempColor) +
			lipgloss.NewStyle().Foreground(tempColor(hi)).Render(fmt.Sprintf(" %-5s", fmt.Sprintf("%.0f°", hi)))
		line += faint().Render(" rain ") + rainCell(d.RainIn, 0)
		if d.PeakGustMph != nil {
			line += faint().Render("  gust ") + fmt.Sprintf("%.0f", *d.PeakGustMph)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", weekStats(days))
	return strings.Join(lines, "\n")
}

// rainCell prints a daily rain amount width columns wide (0 = natural width),
// dimming the zeros so wet days pop. Padding happens before styling: a %*s
// over the styled string would count the ANSI escapes as width and pad nothing.
func rainCell(in float64, width int) string {
	s := fmt.Sprintf("%*.2f", width, in)
	if in == 0 {
		return faint().Render(s)
	}
	return s
}

// orFallback returns *a when present, else *b (callers guarantee one is set).
func orFallback(a, b *float64) float64 {
	if a != nil {
		return *a
	}
	return *b
}

// weekStats aggregates the week into one summary row.
func weekStats(days []store.DayStat) string {
	var hi, lo, gust *float64
	rain, rainy := 0.0, 0
	for _, d := range days {
		hi, lo = maxPtrVal(hi, d.TempMaxF), minPtrVal(lo, d.TempMinF)
		gust = maxPtrVal(gust, d.PeakGustMph)
		rain += d.RainIn
		if d.RainIn >= 0.01 {
			rainy++
		}
	}
	return statRow([]string{
		valF(hi, "hi %.0f°"), valF(lo, "lo %.0f°"),
		fmt.Sprintf("rain %.2f in", rain),
		pluralN(rainy, "rainy day"),
		valF(gust, "gust %.0f mph"),
	})
}

// ---- month view ----------------------------------------------------------------

// renderMonthView draws a Monday-first calendar heatmap tinted by the active
// metric, beside the month's aggregate stats and a color legend.
func renderMonthView(days []store.DayStat, offset int, metric heatMetric) string {
	start, end, _ := periodRange(viewMonth, offset, time.Now())
	if len(days) == 0 {
		return emptyView("no observations this month")
	}
	byDate := map[string]store.DayStat{}
	var vals []*float64
	for _, d := range days {
		byDate[d.Day] = d
		vals = append(vals, metric.value(d))
	}
	lo, hi, haveVals := minMaxVals(vals)

	// The calendar grid, one 3-cell column per weekday.
	var rows []string
	rows = append(rows, faint().Render("Mo Tu We Th Fr Sa Su"))
	row := make([]string, 0, 7)
	pad := (int(start.Weekday()) + 6) % 7
	for range pad {
		row = append(row, "  ")
	}
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		row = append(row, monthCell(byDate, day, metric, lo, hi))
		if len(row) == 7 {
			rows = append(rows, strings.Join(row, " "))
			row = row[:0]
		}
	}
	if len(row) > 0 {
		rows = append(rows, strings.Join(row, " "))
	}
	grid := strings.Join(rows, "\n")

	// The stats column beside it.
	var stats []string
	stats = append(stats, faint().Render("metric ")+lipgloss.NewStyle().Bold(true).Render(metric.name)+faint().Render(" (tab)"))
	if haveVals {
		stats = append(stats, legendBar(metric, lo, hi))
	}
	stats = append(stats, "")
	var mhi, mlo, mgust *float64
	rain, rainy, observed := 0.0, 0, 0
	for _, d := range days {
		mhi, mlo = maxPtrVal(mhi, d.TempMaxF), minPtrVal(mlo, d.TempMinF)
		mgust = maxPtrVal(mgust, d.PeakGustMph)
		rain += d.RainIn
		if d.RainIn >= 0.01 {
			rainy++
		}
		if d.Obs > 0 {
			observed++
		}
	}
	stats = append(stats,
		statRow([]string{valF(mhi, "hi %.0f°"), valF(mlo, "lo %.0f°")}),
		statRow([]string{fmt.Sprintf("rain %.2f in", rain), pluralN(rainy, "rainy day")}),
		statRow([]string{valF(mgust, "gust %.0f mph")}),
		statRow([]string{fmt.Sprintf("observed %d/%d days", observed, daysIn(start, end))}),
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, grid, "    ",
		lipgloss.JoinVertical(lipgloss.Left, stats...))
}

// monthCell renders one calendar day as a 2-wide heat cell: colored when the
// metric has a value, dim dots when the archive has nothing for that day, and
// blank for days still in the future.
func monthCell(byDate map[string]store.DayStat, day time.Time, metric heatMetric, lo, hi float64) string {
	if day.After(time.Now()) {
		return "  "
	}
	d, have := byDate[day.Format("2006-01-02")]
	if !have {
		return faint().Render("··")
	}
	v := metric.value(d)
	if v == nil {
		return faint().Render("··")
	}
	return lipgloss.NewStyle().Foreground(metric.color(*v, lo, hi)).Render("██")
}

// legendBar shows the metric's color ramp with its endpoints labeled.
func legendBar(metric heatMetric, lo, hi float64) string {
	const cells = 8
	var b strings.Builder
	for i := range cells {
		v := lo + (hi-lo)*float64(i)/float64(cells-1)
		b.WriteString(lipgloss.NewStyle().Foreground(metric.color(v, lo, hi)).Render("█"))
	}
	return faint().Render(fmt.Sprintf(metric.format, lo)+" ") + b.String() +
		faint().Render(" "+fmt.Sprintf(metric.format, hi))
}

// daysIn counts whole days in [start, end), capped at today for the running
// period so "observed n/m" never counts the future as missing.
func daysIn(start, end time.Time) int {
	if today := midnightOf(time.Now()).AddDate(0, 0, 1); end.After(today) {
		end = today
	}
	return int(end.Sub(start).Hours() / 24)
}

// ---- year view -----------------------------------------------------------------

// renderYearView draws a GitHub-contributions-style ribbon (weeks × weekdays,
// tinted by the active metric) over a monthly climatology strip.
func renderYearView(y yearData, offset int, metric heatMetric) string {
	start, end, _ := periodRange(viewYear, offset, time.Now())
	if len(y.days) == 0 {
		return emptyView("no observations this year")
	}
	byDate := map[string]store.DayStat{}
	var vals []*float64
	for _, d := range y.days {
		byDate[d.Day] = d
		vals = append(vals, metric.value(d))
	}
	lo, hi, _ := minMaxVals(vals)

	firstMonday := mondayOf(start)
	const cols = 54
	grid := make([][]string, 7)
	for r := range grid {
		grid[r] = make([]string, cols)
		for c := range grid[r] {
			grid[r][c] = " "
		}
	}
	monthMarks := make([]string, cols)
	for c := range monthMarks {
		monthMarks[c] = " "
	}
	now := time.Now()
	for day := start; day.Before(end) && !day.After(now); day = day.AddDate(0, 0, 1) {
		col := daysBetween(firstMonday, day) / 7
		if col < 0 || col >= cols {
			continue
		}
		if day.Day() == 1 || day.Equal(start) {
			monthMarks[col] = day.Format("Jan")[:1]
		}
		row := (int(day.Weekday()) + 6) % 7
		cell := faint().Render("·")
		if d, have := byDate[day.Format("2006-01-02")]; have {
			if v := metric.value(d); v != nil {
				cell = lipgloss.NewStyle().Foreground(metric.color(*v, lo, hi)).Render("█")
			}
		}
		grid[row][col] = cell
	}

	gutter := [7]string{"Mon", "", "Wed", "", "Fri", "", ""}
	var lines []string
	lines = append(lines, strings.Repeat(" ", 4)+faint().Render(strings.Join(monthMarks, "")))
	for r := range grid {
		lines = append(lines, faint().Render(fmt.Sprintf("%-4s", gutter[r]))+strings.Join(grid[r], ""))
	}

	lines = append(lines, "",
		faint().Render("metric ")+lipgloss.NewStyle().Bold(true).Render(metric.name)+
			faint().Render(" (tab)")+"  "+legendBar(metric, lo, hi),
		"", monthStrip(y.months, start), "", yearStats(y.days))
	return strings.Join(lines, "\n")
}

// monthStrip is the two-row monthly climatology under the ribbon: average
// temperature as colored cells, rainfall as a scaled mini-bar per month.
func monthStrip(months []store.PeriodStat, start time.Time) string {
	byMonth := map[string]store.PeriodStat{}
	rainHi := 0.0
	for _, p := range months {
		byMonth[p.Period] = p
		rainHi = max(rainHi, p.RainIn)
	}
	head, temp, rain := strings.Repeat(" ", 6), faint().Render(fmt.Sprintf("%-6s", "temp")), faint().Render(fmt.Sprintf("%-6s", "rain"))
	for m := 1; m <= 12; m++ {
		key := fmt.Sprintf("%04d-%02d", start.Year(), m)
		head += faint().Render(time.Month(m).String()[:1] + "   ")
		p, have := byMonth[key]
		if !have {
			temp += faint().Render(gapDot) + "   "
			rain += faint().Render(gapDot) + "   "
			continue
		}
		if p.TempAvgF != nil {
			temp += lipgloss.NewStyle().Foreground(tempColor(*p.TempAvgF)).Render("██") + "  "
		} else {
			temp += faint().Render(gapDot) + "   "
		}
		idx := 0
		if rainHi > 0 {
			idx = int(norm(p.RainIn, 0, rainHi) * float64(len(blocks8)-1))
		}
		rain += lipgloss.NewStyle().Foreground(rainColor(p.RainIn, 0, rainHi)).
			Render(string(blocks8[idx])) + "   "
	}
	return strings.Join([]string{head, temp, rain}, "\n")
}

// yearStats aggregates the year into one summary row.
func yearStats(days []store.DayStat) string {
	var hi, lo, gust *float64
	rain := 0.0
	for _, d := range days {
		hi, lo = maxPtrVal(hi, d.TempMaxF), minPtrVal(lo, d.TempMinF)
		gust = maxPtrVal(gust, d.PeakGustMph)
		rain += d.RainIn
	}
	return statRow([]string{
		valF(hi, "hi %.0f°"), valF(lo, "lo %.0f°"),
		fmt.Sprintf("rain %.2f in", rain),
		valF(gust, "gust %.0f mph"),
	})
}

// ---- records view --------------------------------------------------------------

// renderRecordsView draws the all-time records board plus today's calendar day
// across every archived year.
func renderRecordsView(rd recordsData) string {
	r := rd.records
	bold := lipgloss.NewStyle().Bold(true)
	var lines []string
	lines = append(lines, bold.Render("all-time records"), "")

	add := func(label, value string, color lipgloss.Color, when string) {
		line := faint().Render(fmt.Sprintf("  %-13s", label)) +
			lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("%-11s", value))
		if when != "" {
			line += faint().Render(when)
		}
		lines = append(lines, line)
	}
	if r.HottestF != nil {
		add("hottest", fmt.Sprintf("%.1f °F", *r.HottestF), tempColor(*r.HottestF), epochShort(r.HottestEpoch))
	}
	if r.ColdestF != nil {
		add("coldest", fmt.Sprintf("%.1f °F", *r.ColdestF), tempColor(*r.ColdestF), epochShort(r.ColdestEpoch))
	}
	if r.PeakGustMph != nil {
		add("peak gust", fmt.Sprintf("%.0f mph", *r.PeakGustMph), gustColor(1, 0, 1), epochShort(r.PeakGustEpoch))
	}
	if r.LowestPressureInHg != nil {
		add("low pressure", fmt.Sprintf("%.2f inHg", *r.LowestPressureInHg), lipgloss.Color("#4FC3F7"), epochShort(r.LowestPressureEpoch))
	}
	if r.WettestDayIn != nil {
		when := r.WettestDay
		if t, err := time.ParseInLocation("2006-01-02", r.WettestDay, time.Local); err == nil {
			when = t.Format("Jan 2 2006") // match the other records' date style
		}
		add("wettest day", fmt.Sprintf("%.2f in", *r.WettestDayIn), rainColor(1, 0, 1), when)
	}
	if r.TotalStrikes != nil && *r.TotalStrikes > 0 {
		add("lightning", fmt.Sprintf("%.0f", *r.TotalStrikes), lipgloss.Color("#FFD23F"), "strikes all-time")
	}
	if r.PeakSolarWm2 != nil {
		add("peak solar", fmt.Sprintf("%.0f W/m²", *r.PeakSolarWm2), lipgloss.Color("#FF9F1C"), epochShort(r.PeakSolarEpoch))
	}
	if r.PeakUV != nil {
		add("peak UV", fmt.Sprintf("%.1f", *r.PeakUV), lipgloss.Color("#9D4EDD"), epochShort(r.PeakUVEpoch))
	}

	if len(rd.thisDay) > 0 {
		lines = append(lines, "",
			bold.Render(time.Now().Format("Jan 2")+" in history"), "",
			faint().Render(fmt.Sprintf("  %-6s %6s %6s %8s", "year", "hi", "lo", "rain")))
		var hiRec *float64
		hiYear := 0
		for _, yd := range rd.thisDay {
			if yd.TempMaxF != nil && (hiRec == nil || *yd.TempMaxF > *hiRec) {
				hiRec, hiYear = yd.TempMaxF, yd.Year
			}
		}
		for _, yd := range rd.thisDay {
			line := fmt.Sprintf("  %-6d %s %s %s",
				yd.Year, tempCell(yd.TempMaxF), tempCell(yd.TempMinF), rainCell(yd.RainIn, 8))
			if yd.Year == hiYear && hiRec != nil {
				line += faint().Render("   ← record high")
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// tempCell renders a nullable °F value 6 columns wide, tinted on the ramp.
func tempCell(v *float64) string {
	if v == nil {
		return faint().Render(fmt.Sprintf("%6s", "--"))
	}
	return lipgloss.NewStyle().Foreground(tempColor(*v)).Render(fmt.Sprintf("%5.0f°", *v))
}

// epochShort renders a record's timestamp like "Aug 14 2024 15:03".
func epochShort(e *int64) string {
	if e == nil {
		return ""
	}
	return time.Unix(*e, 0).Local().Format("Jan 2 2006 15:04")
}

// ---- shared helpers --------------------------------------------------------------

// statRow joins readings with a slim faint middot, dropping empties, like
// render.go's secondary() but tight enough for the explorer's longer rows.
func statRow(parts []string) string {
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return faint().Render(strings.Join(kept, " · "))
}

// emptyView is the placeholder body for a period with no archived data.
func emptyView(msg string) string {
	return "\n" + lipgloss.NewStyle().Faint(true).Width(exploreWidth).
		Align(lipgloss.Center).Render("· "+msg+" ·") + "\n"
}

// minPtrVal / maxPtrVal merge nullable readings (nil = no reading yet), the
// display-side twins of the store's rollup helpers.
func minPtrVal(a, b *float64) *float64 {
	if a == nil || (b != nil && *b < *a) {
		return b
	}
	return a
}

func maxPtrVal(a, b *float64) *float64 {
	if a == nil || (b != nil && *b > *a) {
		return b
	}
	return a
}

// pluralN renders "1 rainy day" / "3 rainy days"; "" when n is zero so
// statRow drops it.
func pluralN(n int, noun string) string {
	switch n {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("1 %s", noun)
	default:
		return fmt.Sprintf("%d %ss", n, noun)
	}
}
