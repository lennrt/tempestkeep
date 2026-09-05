package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func assertFrameBounds(t *testing.T, frame string, width, height int) {
	t.Helper()
	if !utf8.ValidString(frame) {
		t.Fatal("frame contains invalid UTF-8")
	}
	if width <= 0 || height <= 0 {
		if frame != "" {
			t.Fatal("frame painted before a usable size was received")
		}
		return
	}
	if got := lineCount(frame); got > height {
		t.Fatalf("frame height %d exceeds terminal height %d:\n%s", got, height, frame)
	}
	for line := range strings.SplitSeq(frame, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line width %d exceeds terminal width %d: %q", got, width, line)
		}
	}
}

func TestFitLineAndSpreadWidths(t *testing.T) {
	labels := []string{"", "station", strings.Repeat("weather", 20), "海辺の気象台", "e\u0301e\u0301e\u0301", "\x1b[31mcolored station\x1b[0m"}
	for width := range 71 {
		for _, left := range labels {
			got := fitLine(left, width)
			if !utf8.ValidString(got) || lipgloss.Width(got) > width {
				t.Fatalf("fitLine(%q, %d) = %q", left, width, got)
			}
			for _, right := range labels {
				got := spread(left, right, width)
				if !utf8.ValidString(got) || lipgloss.Width(got) != width {
					t.Fatalf("spread(%q, %q, %d) = %q (width %d)", left, right, width, got, lipgloss.Width(got))
				}
			}
		}
	}
}

func TestTerminalFrameBoundsAndScrolling(t *testing.T) {
	rows := []string{"first", "second", "third", "fourth", "last"}
	body := strings.Join(rows, "\n")
	for _, size := range [][2]int{{0, 0}, {1, 1}, {2, 2}, {20, 3}, {80, 24}} {
		for _, offset := range []int{-99, 0, 1, 99} {
			frame := terminalFrame(body, size[0], size[1], offset)
			assertFrameBounds(t, frame, size[0], size[1])
		}
	}
	if got := terminalFrame(body, 60, 3, 0); !strings.Contains(got, "first") || !strings.Contains(got, "scroll") || strings.Contains(got, "last") {
		t.Fatalf("top of overflowing frame is incorrect:\n%s", got)
	}
	if got := terminalFrame(body, 60, 3, 999); !strings.Contains(got, "last") || strings.Contains(got, "first") {
		t.Fatalf("bottom of overflowing frame is incorrect:\n%s", got)
	}
	if got := terminalFrame(body, 60, 24, 999); !strings.Contains(got, "first") || strings.Contains(got, "scroll") {
		t.Fatalf("fitting frame was scrolled or given unnecessary help:\n%s", got)
	}
	assertFrameBounds(t, terminalFrame("\x1b[31m"+strings.Repeat("海", 40)+"\x1b[0m", 20, 2, 0), 20, 2)
}

func TestForecastStripPartialAndEmpty(t *testing.T) {
	if got := forecastStrip(nil); got != "" {
		t.Fatalf("empty forecast = %q, want empty", got)
	}
	for count := 1; count <= 9; count++ {
		cells := make([]dailyCell, count)
		for i := range cells {
			cells[i] = dailyCell{label: strings.Repeat("海辺", 20), icon: "*"}
		}
		got := forecastStrip(cells)
		if lipgloss.Width(got) != contentWidth {
			t.Errorf("%d forecast cells use %d columns, want %d", count, lipgloss.Width(got), contentWidth)
		}
		if count > 5 && got != forecastStrip(cells[:5]) {
			t.Errorf("forecast did not limit itself to five cells")
		}
	}
}
