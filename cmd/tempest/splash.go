package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// brandGradient colors each rune of s along a sky-to-sun gradient, the one
// splash of flair shared by the `now` splash screen and the setup wizard.
func brandGradient(s string) string {
	from, _ := colorful.Hex("#5B9BD5") // sky blue
	to, _ := colorful.Hex("#FFD23F")   // sunshine yellow
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range runes {
		t := float64(i) / float64(max(len(runes)-1, 1))
		c := from.BlendLuv(to, t)
		b.WriteString(lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(c.Hex())).Render(string(r)))
	}
	return b.String()
}

// splashArt renders the gradient wordmark with its tagline beneath, the single
// brand mark shared by the `now` splash, the `explore` splash, and the setup
// wizard header. One wordmark everywhere keeps the tools visually consistent.
func splashArt() string {
	title := brandGradient("TEMPESTKEEP")
	tag := faint().Render("local weather · lasting history")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", tag)
}
