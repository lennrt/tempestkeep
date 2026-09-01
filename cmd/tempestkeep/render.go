package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/api"
	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

// contentWidth divides into five 11-column forecast cells.
const contentWidth = 55

// nowMinWidth includes the border and two columns of padding on each side.
const nowMinWidth = contentWidth + 2 + 4

// narrowNotice replaces a card that would wrap in a narrow terminal.
func narrowNotice(have, need int) string {
	return faint().Width(have).Render(
		fmt.Sprintf("Terminal is %d columns wide; widen to at least %d for the dashboard.", have, need))
}

// dashboard contains the data needed by both live and archive views.
type dashboard struct {
	station    string
	source     string // "live" or "archive"
	obsTime    time.Time
	conditions string
	icon       string

	tempF        *float64
	feelsF       *float64
	humidityPct  *float64
	dewF         *float64
	pressureInHg *float64
	windMph      *float64
	gustMph      *float64
	windDirDeg   *float64
	uv           *float64
	solarWm2     *float64
	rainTodayIn  *float64
	lightning1h  *int
	note         string

	daily []dailyCell
}

type dailyCell struct {
	label      string // weekday, e.g. "Today", "Sat"
	icon       string
	conditions string
	hiF        *float64
	loF        *float64
}

// ---- builders ---------------------------------------------------------------

// buildLiveDashboard combines the precise latest station observation (sensor
// values) with the forecast (condition icon + daily strip) into one view.
func buildLiveDashboard(station *api.Station, o *api.StationObs, fc *api.Forecast) dashboard {
	d := dashboard{source: "live", obsTime: time.Unix(o.Timestamp, 0)}
	if station != nil {
		d.station = station.Name
	}
	if fc != nil {
		d.conditions = fc.CurrentConditions.Conditions
		d.icon = fc.CurrentConditions.Icon
		for i, day := range fc.Forecast.Daily {
			if i >= 5 {
				break
			}
			d.daily = append(d.daily, dailyCell{
				label:      weekdayLabel(day.DayStartLocal, i),
				icon:       glyphFor(day.Icon, day.Conditions),
				conditions: day.Conditions,
				hiF:        cToFptr(day.AirTempHigh),
				loF:        cToFptr(day.AirTempLow),
			})
		}
	}
	if o.AirTemperature != nil {
		d.tempF = new(model.CToF(*o.AirTemperature))
	}
	if o.FeelsLike != nil {
		d.feelsF = new(model.CToF(*o.FeelsLike))
	}
	if o.DewPoint != nil {
		d.dewF = new(model.CToF(*o.DewPoint))
	}
	d.humidityPct = o.RelativeHumidity
	if p := firstNonNil(o.SeaLevelPressure, o.StationPressure); p != nil {
		d.pressureInHg = new(model.MbToInHg(*p))
	}
	if o.WindAvg != nil {
		d.windMph = new(model.MpsToMph(*o.WindAvg))
	}
	if o.WindGust != nil {
		d.gustMph = new(model.MpsToMph(*o.WindGust))
	}
	d.windDirDeg = o.WindDirection
	d.uv = o.UV
	d.solarWm2 = o.SolarRadiation
	if o.PrecipAccumLocalDay != nil {
		d.rainTodayIn = new(model.MmToInch(*o.PrecipAccumLocalDay))
	}
	d.lightning1h = o.LightningLast1hr
	return d
}

// buildArchiveDashboard renders from the newest archived observation when no
// token is configured. Feels-like and dew point are derived; there is no forecast.
func buildArchiveDashboard(station string, o *model.Obs) dashboard {
	d := dashboard{source: "archive", station: station, obsTime: time.Unix(o.Epoch, 0),
		note: "from local archive: no live forecast; pressure is station, not sea-level"}
	if o.AirTempC != nil {
		tF := model.CToF(*o.AirTempC)
		d.tempF = new(tF)
		if o.HumidityPct != nil {
			if dp := model.DewPointC(*o.AirTempC, *o.HumidityPct); !math.IsNaN(dp) {
				d.dewF = new(model.CToF(dp))
			}
		}
		switch {
		case tF >= 80 && o.HumidityPct != nil:
			d.feelsF = new(model.ApparentTempF(tF, *o.HumidityPct, 0))
		case tF <= 50 && o.WindAvgMps != nil:
			d.feelsF = new(model.ApparentTempF(tF, 0, model.MpsToMph(*o.WindAvgMps)))
		case tF > 50 && tF < 80:
			d.feelsF = new(tF)
		}
	}
	d.humidityPct = o.HumidityPct
	if o.PressureMb != nil {
		d.pressureInHg = new(model.MbToInHg(*o.PressureMb))
	}
	if o.WindAvgMps != nil {
		d.windMph = new(model.MpsToMph(*o.WindAvgMps))
	}
	if o.WindGustMps != nil {
		d.gustMph = new(model.MpsToMph(*o.WindGustMps))
	}
	d.windDirDeg = o.WindDirDeg
	d.uv = o.UV
	d.solarWm2 = o.SolarWm2
	return d
}

// ---- rendering --------------------------------------------------------------

// renderDashboard draws one card. footer is optional key help.
func renderDashboard(d dashboard, now time.Time, footer string) string {
	art := artFor(d.icon, d.conditions)
	accent := lipgloss.Color(art.accent)

	sections := []string{
		header(d, now),
		divider(),
		lipgloss.JoinHorizontal(lipgloss.Top, artBlock(art), "  ", infoBlock(d, accent)),
	}
	if len(d.daily) > 0 {
		sections = append(sections, divider(), forecastStrip(d.daily))
	}
	if d.note != "" {
		sections = append(sections, faint().Width(contentWidth).Render(d.note))
	}
	if footer != "" {
		sections = append(sections, divider(), faint().Render(footer))
	}

	card := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 2).
		Render(card)
}

func header(d dashboard, now time.Time) string {
	name := d.station
	if name == "" {
		name = "Tempest"
	}
	// No emoji here: lipgloss measures ⛅ at 2 cells but many terminals draw
	// it at 1, which pulls this line's right border out of column.
	left := lipgloss.NewStyle().Bold(true).Render(name)
	badge := d.source
	if age := now.Sub(d.obsTime); age > 0 {
		badge = fmt.Sprintf("%s · %s ago", d.source, humanizeAge(age))
	}
	right := faint().Render(now.Format("Mon 15:04") + " · " + badge)
	return spread(left, right, contentWidth)
}

// infoBlock is the five-line readout beside the art: conditions, temperature,
// wind, and two rows of secondary readings.
func infoBlock(d dashboard, accent lipgloss.Color) string {
	bold := lipgloss.NewStyle().Bold(true)

	conditions := d.conditions
	if conditions == "" {
		conditions = capitalize(d.source) + " reading"
	}
	line1 := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(conditions)

	temp := "  --  "
	if d.tempF != nil {
		temp = lipgloss.NewStyle().Foreground(tempColor(*d.tempF)).Bold(true).
			Render(fmt.Sprintf("%.0f°F", *d.tempF))
	}
	feels := ""
	if d.feelsF != nil {
		feels = faint().Render(fmt.Sprintf("  feels %.0f°F", *d.feelsF))
	}
	line2 := temp + feels

	line3 := bold.Render(windString(d))
	line4 := secondary([]string{
		valF(d.humidityPct, "%.0f%% RH"),
		valF(d.dewF, "dew %.0f°F"),
		valF(d.pressureInHg, "%.2f inHg"),
	})
	line5 := secondary([]string{
		valF(d.uv, "UV %.0f"),
		valF(d.solarWm2, "%.0f W/m²"),
		valF(d.rainTodayIn, "rain %.2f in"),
	})
	if d.lightning1h != nil && *d.lightning1h > 0 {
		// ⚡ has the same unstable width as ⛅ (see header), and UV means
		// nothing mid-storm anyway, so the storm variant is lightning + rain.
		line5 = secondary([]string{
			fmt.Sprintf("lightning %d/hr", *d.lightning1h),
			valF(d.rainTodayIn, "rain %.2f in"),
		})
	}
	return lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3, line4, line5)
}

// padGlyph normalizes a weather glyph to two display columns so forecast cells
// align regardless of whether a given emoji measures as one cell or two.
func padGlyph(g string) string {
	if lipgloss.Width(g) < 2 {
		return g + " "
	}
	return g
}

func artBlock(a weatherArt) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(a.accent)).
		Render(strings.Join(a.lines, "\n"))
}

// forecastStrip lays the daily cells out as equal-width columns.
func forecastStrip(cells []dailyCell) string {
	cellW := contentWidth / len(cells)
	rendered := make([]string, 0, len(cells))
	for _, c := range cells {
		hilo := "  --  "
		if c.hiF != nil && c.loF != nil {
			hi := lipgloss.NewStyle().Foreground(tempColor(*c.hiF)).Render(fmt.Sprintf("%.0f°", *c.hiF))
			lo := lipgloss.NewStyle().Foreground(tempColor(*c.loF)).Faint(true).Render(fmt.Sprintf("%.0f°", *c.loF))
			hilo = hi + "/" + lo
		}
		body := lipgloss.JoinVertical(lipgloss.Center,
			faint().Render(c.label),
			padGlyph(c.icon),
			hilo,
		)
		rendered = append(rendered, lipgloss.NewStyle().Width(cellW).Align(lipgloss.Center).Render(body))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

// ---- small styling helpers --------------------------------------------------

func faint() lipgloss.Style { return lipgloss.NewStyle().Faint(true) }

func divider() string { return faint().Render(dividerLine(contentWidth)) }

func dividerLine(w int) string { return strings.Repeat("─", w) }

// spread places left and right on one line separated to fill width.
func spread(left, right string, width int) string {
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// secondary joins a row of readings with a faint middle dot, dropping empties.
func secondary(parts []string) string {
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, faint().Render("  ·  "))
}

func windString(d dashboard) string {
	if d.windMph == nil {
		return "wind  --"
	}
	arrow, compass := "•", ""
	if d.windDirDeg != nil {
		arrow = windArrow(*d.windDirDeg)
		compass = " " + model.Compass(*d.windDirDeg)
	}
	s := fmt.Sprintf("%s%s  %.0f mph", arrow, compass, *d.windMph)
	if d.gustMph != nil {
		s += fmt.Sprintf("  ·  gust %.0f", *d.gustMph)
	}
	return s
}

// windArrow returns an 8-point arrow pointing the way the wind travels (toward),
// given the meteorological direction it blows from. That is the wttr.in
// convention, so a north wind (from N) shows ↓.
func windArrow(fromDeg float64) string {
	if math.IsNaN(fromDeg) || math.IsInf(fromDeg, 0) {
		return "•"
	}
	arrows := [8]string{"↑", "↗", "→", "↘", "↓", "↙", "←", "↖"}
	travel := math.Mod(fromDeg+180, 360)
	if travel < 0 {
		travel += 360
	}
	return arrows[int(travel/45+0.5)%8]
}

func valF(v *float64, format string) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf(format, *v)
}

// tempColor maps a Fahrenheit temperature onto a cold→hot color ramp so the
// number reads at a glance.
func tempColor(f float64) lipgloss.Color {
	switch {
	case f < 20:
		return lipgloss.Color("#6FB1FF") // icy blue
	case f < 35:
		return lipgloss.Color("#7FD3E8") // cold cyan
	case f < 50:
		return lipgloss.Color("#7FD1AE") // cool teal
	case f < 65:
		return lipgloss.Color("#A6D96A") // mild green
	case f < 78:
		return lipgloss.Color("#FFD23F") // warm yellow
	case f < 90:
		return lipgloss.Color("#FB8B24") // hot orange
	default:
		return lipgloss.Color("#E63946") // scorching red
	}
}

func weekdayLabel(dayStartLocal int64, i int) string {
	if i == 0 {
		return "Today"
	}
	return time.Unix(dayStartLocal, 0).Local().Format("Mon")
}

func humanizeAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

//go:fix inline
func cToFptr(c *float64) *float64 {
	if c == nil {
		return nil
	}
	v := model.CToF(*c)
	return &v
}

func firstNonNil(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}
