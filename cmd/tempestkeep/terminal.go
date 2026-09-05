package main

import (
	"strings"
	"unicode"
)

// scrollRange reserves one row for navigation help only when content overflows.
// A one-row terminal still displays one content row and remains scrollable.
func scrollRange(rows, height, offset int) (start, end int) {
	if rows <= 0 || height <= 0 {
		return 0, 0
	}
	visible := height
	if rows > height && height > 1 {
		visible--
	}
	start = min(max(offset, 0), max(rows-visible, 0))
	return start, min(start+visible, rows)
}

// scrollOffset handles only scrolling keys; the caller keeps its existing
// navigation bindings (in particular, the explorer's Home means latest period).
func scrollOffset(key string, offset, rows, height int) (int, bool) {
	start, end := scrollRange(rows, height, offset)
	page := max(end-start-1, 1)
	var delta int
	switch key {
	case "up", "k":
		delta = -1
	case "down", "j":
		delta = 1
	case "pgup":
		delta = -page
	case "pgdown":
		delta = page
	default:
		return offset, false
	}
	start, _ = scrollRange(rows, height, start+delta)
	return start, true
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// displayText normalizes an external label to one line without terminal control
// characters. Do this before styling, not on a string containing our own ANSI.
func displayText(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
}
