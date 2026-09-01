package main

// Chart primitives for `tempestkeep explore`. Everything renders with the
// eighth-block glyphs (▁▂▃▄▅▆▇█), which every modern monospaced font carries,
// so the explorer needs no plotting dependency and no NerdFont. Values are
// nullable (*float64) throughout because archive days can be partial: a nil
// renders as a dim dot rather than a misleading zero.

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// blocks8 are the eight partial-height column glyphs, shortest first.
var blocks8 = []rune("▁▂▃▄▅▆▇█")

// gapDot is how every chart marks a slot with no observation.
const gapDot = "·"

// chartColor picks the color for column i holding value v; charts call it per
// column so a series can be tinted by its own values (e.g. the temp ramp).
type chartColor func(i int, v float64) lipgloss.Color

// minMaxVals scans nullable values and reports the extremes of the non-nil
// ones; ok is false when every value is nil.
func minMaxVals(vals []*float64) (lo, hi float64, ok bool) {
	for _, v := range vals {
		if v == nil {
			continue
		}
		if !ok {
			lo, hi, ok = *v, *v, true
			continue
		}
		lo, hi = math.Min(lo, *v), math.Max(hi, *v)
	}
	return lo, hi, ok
}

// norm maps v onto [0,1] within [lo,hi]; a flat series pins to the middle so
// it still draws at half height instead of vanishing.
func norm(v, lo, hi float64) float64 {
	if hi <= lo {
		return 0.5
	}
	return (v - lo) / (hi - lo)
}

// sparkline renders vals as a single row of eighth-blocks on a zero-floored
// scale (0..max). The zero floor keeps quantity series honest: a rainless day
// sits on the baseline instead of floating at half height. Nil values render
// as a dim gap dot.
func sparkline(vals []*float64, color chartColor) string {
	_, hi, ok := minMaxVals(vals)
	var b strings.Builder
	for i, v := range vals {
		if v == nil || !ok {
			b.WriteString(faint().Render(gapDot))
			continue
		}
		idx := 0
		if hi > 0 {
			idx = int(norm(*v, 0, hi) * float64(len(blocks8)-1))
		}
		b.WriteString(lipgloss.NewStyle().Foreground(color(i, *v)).Render(string(blocks8[idx])))
	}
	return b.String()
}

// columnChart renders vals as a chart `height` rows tall, one column per
// value, using partial blocks for sub-row precision (height rows ⇒ height×8
// distinct levels). It returns the rows top-first; nil values draw a dim gap
// dot on the baseline.
func columnChart(vals []*float64, height int, color chartColor) []string {
	lo, hi, ok := minMaxVals(vals)
	rows := make([]strings.Builder, height)
	for i, v := range vals {
		if v == nil || !ok {
			for r := 0; r < height-1; r++ {
				rows[r].WriteString(" ")
			}
			rows[height-1].WriteString(faint().Render(gapDot))
			continue
		}
		// Total eighths of fill in this column, bottom-up; at least one so a
		// series minimum still leaves a visible mark.
		levels := max(int(math.Round(norm(*v, lo, hi)*float64(height*8))), 1)
		style := lipgloss.NewStyle().Foreground(color(i, *v))
		for r := range rows {
			remaining := levels - (height-1-r)*8
			switch {
			case remaining >= 8:
				rows[r].WriteString(style.Render("█"))
			case remaining >= 1:
				rows[r].WriteString(style.Render(string(blocks8[remaining-1])))
			default:
				rows[r].WriteString(" ")
			}
		}
	}
	out := make([]string, height)
	for r := range rows {
		out[r] = rows[r].String()
	}
	return out
}

// rangeBand draws a horizontal lo..hi band positioned on a fixed [scaleLo,
// scaleHi] axis: dim track outside the band, colored blocks inside (each cell
// tinted by the value it represents, so a hot afternoon glows red while the
// same day's cold dawn stays blue). Used for the week view's daily temp spans.
func rangeBand(lo, hi, scaleLo, scaleHi float64, width int, color func(v float64) lipgloss.Color) string {
	if width < 2 {
		width = 2
	}
	span := scaleHi - scaleLo
	if span <= 0 {
		span = 1
	}
	pos := func(v float64) int {
		p := int(math.Round((v - scaleLo) / span * float64(width-1)))
		return max(0, min(width-1, p))
	}
	from, to := pos(lo), pos(hi)
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < from || i > to {
			b.WriteString(faint().Render("╌"))
			continue
		}
		// The value this cell stands for, interpolated across the band.
		v := lo
		if to > from {
			v = lo + (hi-lo)*float64(i-from)/float64(to-from)
		}
		b.WriteString(lipgloss.NewStyle().Foreground(color(v)).Render("█"))
	}
	return b.String()
}

// ramp blends between two hex colors in Luv space (perceptually smooth) at
// t ∈ [0,1].
func ramp(fromHex, toHex string, t float64) lipgloss.Color {
	from, _ := colorful.Hex(fromHex)
	to, _ := colorful.Hex(toHex)
	return lipgloss.Color(from.BlendLuv(to, max(0, min(1, t))).Hex())
}

// rainColor tints rainfall from a dim slate (trace) to a saturated sky blue
// (the period's wettest), normalized to [lo,hi].
func rainColor(v, lo, hi float64) lipgloss.Color {
	return ramp("#3B4A63", "#6FC3FF", norm(v, lo, hi))
}

// gustColor tints wind gusts from a calm teal to a violent violet, normalized
// to [lo,hi].
func gustColor(v, lo, hi float64) lipgloss.Color {
	return ramp("#7FD1AE", "#C792EA", norm(v, lo, hi))
}

// solarColor tints solar radiation from dusk slate to full-sun yellow on an
// absolute 0..1000 W/m² scale.
func solarColor(v float64) lipgloss.Color {
	return ramp("#4A4458", "#FFD23F", v/1000)
}
