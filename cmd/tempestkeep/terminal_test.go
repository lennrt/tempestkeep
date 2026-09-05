package main

import (
	"strings"
	"testing"
	"unicode"
)

func TestScrollRange(t *testing.T) {
	cases := []struct {
		name                 string
		rows, height, offset int
		wantStart, wantEnd   int
	}{
		{"fits", 8, 10, 4, 0, 8},
		{"exact fit", 10, 10, 9, 0, 10},
		{"reserves help", 20, 10, 0, 0, 9},
		{"middle", 20, 10, 4, 4, 13},
		{"clamps bottom", 20, 10, 100, 11, 20},
		{"clamps top", 20, 10, -100, 0, 9},
		{"one row", 20, 1, 19, 19, 20},
		{"zero rows", 0, 10, 4, 0, 0},
		{"zero height", 20, 0, 4, 0, 0},
		{"negative rows", -1, 10, 4, 0, 0},
		{"negative height", 20, -1, 4, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := scrollRange(tc.rows, tc.height, tc.offset)
			if start != tc.wantStart || end != tc.wantEnd {
				t.Fatalf("scrollRange(%d, %d, %d) = [%d,%d), want [%d,%d)",
					tc.rows, tc.height, tc.offset, start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

func TestScrollKeys(t *testing.T) {
	for key, want := range map[string]int{
		"up": 9, "k": 9, "down": 11, "j": 11, "pgup": 2, "pgdown": 18,
	} {
		if got, handled := scrollOffset(key, 10, 40, 10); !handled || got != want {
			t.Errorf("%s = %d, %t; want %d, true", key, got, handled, want)
		}
	}
	for _, key := range []string{"home", "g", "r", "left", "right", "q", "enter", "tab"} {
		if got, handled := scrollOffset(key, 7, 40, 10); handled || got != 7 {
			t.Errorf("scrolling intercepted existing binding %q", key)
		}
	}
	if got, _ := scrollOffset("up", 999, 20, 10); got != 10 {
		t.Errorf("stale offset should clamp before moving: got %d, want 10", got)
	}
	if got, _ := scrollOffset("down", 19, 20, 1); got != 19 {
		t.Errorf("one-row terminal scrolled beyond its last row: %d", got)
	}
	if got, _ := scrollOffset("pgdown", 0, 3, 10); got != 0 {
		t.Errorf("fitting content should not scroll: %d", got)
	}
}

func checkScrollRange(t *testing.T, rows, height, offset int) {
	t.Helper()
	start, end := scrollRange(rows, height, offset)
	if rows <= 0 || height <= 0 {
		if start != 0 || end != 0 {
			t.Fatalf("nonpositive size: [%d,%d)", start, end)
		}
		return
	}
	visible := min(rows, height)
	if rows > height && height > 1 {
		visible--
	}
	if start < 0 || end < start || end > rows || end-start != visible {
		t.Fatalf("invalid range for rows=%d height=%d offset=%d: [%d,%d), visible=%d",
			rows, height, offset, start, end, visible)
	}
	for _, key := range []string{"up", "down", "pgup", "pgdown"} {
		next, handled := scrollOffset(key, offset, rows, height)
		if !handled || next < 0 || next > rows-visible {
			t.Fatalf("%s produced invalid offset %d", key, next)
		}
	}
}

func TestScrollRangeBoundaries(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	values := []int{-maxInt - 1, -1, 0, 1, 2, 3, 10, 24, 100, maxInt - 1, maxInt}
	for _, rows := range values {
		for _, height := range values {
			for _, offset := range values {
				checkScrollRange(t, rows, height, offset)
			}
		}
	}
}

func FuzzScrollRange(f *testing.F) {
	for _, seed := range [][3]int{{20, 10, 0}, {20, 1, 19}, {0, 0, 0}, {40, 24, -1}} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(checkScrollRange)
}

func TestDisplayText(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"  Back\t yard\nstation\r\n": "Back yard station",
		"海辺\u00a0気象台":                "海辺 気象台",
		"rain\x1b[2J\x07\rwind":      "rain [2J wind",
		"Café e\u0301":               "Café e\u0301",
	}
	for input, want := range cases {
		got := displayText(input)
		if got != want {
			t.Errorf("displayText(%q) = %q, want %q", input, got, want)
		}
		if strings.ContainsFunc(got, unicode.IsControl) {
			t.Errorf("control character survived normalization: %q", got)
		}
	}
}

func TestLineCount(t *testing.T) {
	for input, want := range map[string]int{"": 0, "x": 1, "x\ny": 2, "x\n": 2, "\n": 2} {
		if got := lineCount(input); got != want {
			t.Errorf("lineCount(%q) = %d, want %d", input, got, want)
		}
	}
}
