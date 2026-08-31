package main

import "strings"

// weatherArt is a small ASCII picture of the sky plus the accent color it should
// be drawn in. The art is deliberately ASCII-only and a fixed 5 lines tall so it
// stays monospaced and the dashboard columns line up in any terminal font.
type weatherArt struct {
	lines  []string
	accent string // hex color for lipgloss
}

// Art blocks are 5 lines tall; keep glyphs to ASCII so every terminal renders
// them at one cell each (wide Unicode would break the column alignment).
var (
	// Terminal cells run about twice as tall as they are wide, so a disc that is
	// as many columns as rows renders as a vertical teardrop. Draw the sun body
	// wider than it is tall (7 cols over 3 rows) so it reads as a round disc.
	artClearDay = weatherArt{[]string{
		`    \ | /    `,
		`   .-----.   `,
		` --(     )-- `,
		`   '-----'   `,
		`    / | \    `,
	}, "#FFD23F"}

	artClearNight = weatherArt{[]string{
		`     _____   `,
		`   .'     '. `,
		`  :         :`,
		`   '.     .' `,
		`     '---'   `,
	}, "#C9D1FF"}

	artPartlyDay = weatherArt{[]string{
		`   \  /      `,
		` _ /""'-.    `,
		`   \_(   ).  `,
		`   /(___(__) `,
		`             `,
	}, "#F4C95D"}

	artPartlyNight = weatherArt{[]string{
		`     .--.    `,
		`  .-(    ).  `,
		` (___.__)__) `,
		`    *    .   `,
		`  .    *     `,
	}, "#9AA5CE"}

	artCloudy = weatherArt{[]string{
		`             `,
		`     .--.    `,
		`  .-(    ).  `,
		` (___.__)__) `,
		`             `,
	}, "#B7BEC9"}

	artRain = weatherArt{[]string{
		`     .-.     `,
		`    (   ).   `,
		`   (___(__)  `,
		`    ' ' ' '  `,
		`   ' ' ' '   `,
	}, "#5B9BD5"}

	artThunder = weatherArt{[]string{
		`     .-.     `,
		`    (   ).   `,
		`   (___(__)  `,
		`    /_ /_    `,
		`     / /     `,
	}, "#C792EA"}

	artSnow = weatherArt{[]string{
		`     .-.     `,
		`    (   ).   `,
		`   (___(__)  `,
		`    *  *  *  `,
		`   *  *  *   `,
	}, "#E6F1FF"}

	artSleet = weatherArt{[]string{
		`     .-.     `,
		`    (   ).   `,
		`   (___(__)  `,
		`    ' * ' *  `,
		`   * ' * '   `,
	}, "#8FD3E8"}

	artFog = weatherArt{[]string{
		`             `,
		`  _ - _ - _  `,
		`   - _ - _   `,
		`  _ - _ - _  `,
		`             `,
	}, "#9BA3AE"}

	artWind = weatherArt{[]string{
		`             `,
		`   ~~~~~~~   `,
		`  ~~~~~~~~~  `,
		`   ~~~~~~~   `,
		`             `,
	}, "#7FD1AE"}

	artUnknown = weatherArt{[]string{
		`             `,
		`     .-.     `,
		`    ( ? )    `,
		`     '-'     `,
		`             `,
	}, "#B7BEC9"}
)

// artFor picks the art for a WeatherFlow forecast icon (e.g. "partly-cloudy-day",
// "rainy", "possibly-thunderstorm-night"), falling back to the free-text
// conditions string, then to a neutral placeholder. Matching is by keyword so new
// icon variants still map to something sensible.
func artFor(icon, conditions string) weatherArt {
	s := strings.ToLower(icon)
	if s == "" {
		s = strings.ToLower(conditions)
	}
	night := strings.Contains(s, "night")

	switch {
	case strings.Contains(s, "thunder"), strings.Contains(s, "storm"):
		return artThunder
	case strings.Contains(s, "snow"), strings.Contains(s, "flurr"), strings.Contains(s, "blizzard"):
		return artSnow
	case strings.Contains(s, "sleet"), strings.Contains(s, "hail"), strings.Contains(s, "wintry"), strings.Contains(s, "freezing"):
		return artSleet
	case strings.Contains(s, "rain"), strings.Contains(s, "drizzle"), strings.Contains(s, "shower"):
		return artRain
	case strings.Contains(s, "fog"), strings.Contains(s, "mist"), strings.Contains(s, "haze"):
		return artFog
	case strings.Contains(s, "wind"), strings.Contains(s, "breez"):
		return artWind
	case strings.Contains(s, "partly"), strings.Contains(s, "mostly"), strings.Contains(s, "few clouds"):
		if night {
			return artPartlyNight
		}
		return artPartlyDay
	case strings.Contains(s, "cloud"), strings.Contains(s, "overcast"):
		return artCloudy
	case strings.Contains(s, "clear"), strings.Contains(s, "sunny"), strings.Contains(s, "fair"):
		if night {
			return artClearNight
		}
		return artClearDay
	default:
		return artUnknown
	}
}

// glyphFor returns a compact glyph for the forecast strip, where a full art
// block would be too big. Every glyph here must be a BMP text-presentation
// symbol that both measures 1 cell under lipgloss.Width and draws at 1 cell:
// pictographs like ⛅, 🌧, or ⛈ measure and draw at different widths depending
// on the terminal's font fallback, and any mismatch knocks the card border out
// of column. The caller pads each glyph to 2 cells.
func glyphFor(icon, conditions string) string {
	s := strings.ToLower(icon)
	if s == "" {
		s = strings.ToLower(conditions)
	}
	night := strings.Contains(s, "night")
	switch {
	case strings.Contains(s, "thunder"), strings.Contains(s, "storm"):
		return "↯"
	case strings.Contains(s, "snow"), strings.Contains(s, "flurr"), strings.Contains(s, "blizzard"):
		return "❄"
	case strings.Contains(s, "sleet"), strings.Contains(s, "hail"), strings.Contains(s, "wintry"), strings.Contains(s, "freezing"):
		return "❆"
	case strings.Contains(s, "rain"), strings.Contains(s, "drizzle"), strings.Contains(s, "shower"):
		return "☂"
	case strings.Contains(s, "fog"), strings.Contains(s, "mist"), strings.Contains(s, "haze"):
		return "≡"
	case strings.Contains(s, "wind"), strings.Contains(s, "breez"):
		return "≈"
	case strings.Contains(s, "partly"), strings.Contains(s, "mostly"):
		if night {
			return "☁"
		}
		return "☼"
	case strings.Contains(s, "cloud"), strings.Contains(s, "overcast"):
		return "☁"
	case strings.Contains(s, "clear"), strings.Contains(s, "sunny"), strings.Contains(s, "fair"):
		if night {
			return "☾"
		}
		return "☀"
	default:
		return "·"
	}
}
