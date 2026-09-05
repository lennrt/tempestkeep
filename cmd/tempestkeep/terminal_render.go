package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// terminalFrame bounds every state, including loading and errors. Cards that
// fit remain centered; taller cards are scrollable rather than cut off by the
// alt-screen renderer. Until the first size event, don't paint a guessed frame.
func terminalFrame(body string, width, height, offset int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	body = lipgloss.NewStyle().MaxWidth(width).Render(body)
	rows := strings.Split(body, "\n")
	if len(rows) <= height {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
	}
	start, end := scrollRange(len(rows), height, offset)
	visible := strings.Join(rows[start:end], "\n")
	if height > 1 {
		help := fmt.Sprintf("↑/↓ scroll · PgUp/PgDn · q quit  %d–%d/%d", start+1, end, len(rows))
		visible = lipgloss.JoinVertical(lipgloss.Center, visible, faint().Render(fitLine(help, width)))
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, visible)
}

// fitLine truncates by display cells, not bytes, and preserves ANSI styling.
func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return lipgloss.NewStyle().MaxWidth(width-1).Render(s) + "…"
}
