package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lennrt/tempestkeep/pkg/tempest/store"
)

func TestNowFramesFitAtEveryStage(t *testing.T) {
	d := dashboard{
		station: strings.Repeat("海辺 weather station ", 15), source: "live",
		conditions: strings.Repeat("Thunderstorms and heavy rainfall ", 8),
		obsTime:    time.Now(), icon: "rainy", note: strings.Repeat("forecast unavailable ", 15),
		daily: []dailyCell{{label: "Today", icon: "*"}},
	}
	for _, size := range [][2]int{{0, 0}, {1, 1}, {20, 3}, {60, 24}, {61, 24}, {80, 10}, {80, 24}, {120, 40}} {
		for _, stage := range []string{"loading", "error", "data", "refresh error"} {
			m := newNowModel(nowConfig{}, time.Minute)
			m.width, m.height = size[0], size[1]
			switch stage {
			case "error":
				m.loading = false
				m.err = errors.New(strings.Repeat("failed to read station data ", 15))
			case "data", "refresh error":
				m.haveData, m.loading, m.d = true, false, d
				if stage == "refresh error" {
					m.err = errors.New("request failed")
				}
			}
			assertFrameBounds(t, m.View(), size[0], size[1])
		}
	}
	if got := lipgloss.Width(renderDashboard(d, time.Now(), "r refresh · q quit · last update failed, showing cached")); got != nowMinWidth {
		t.Fatalf("long content expanded card to %d columns, want %d", got, nowMinWidth)
	}
}

func TestNowScrollAndResize(t *testing.T) {
	m := newNowModel(nowConfig{}, time.Minute)
	m.haveData, m.loading = true, false
	m.width, m.height = 80, 5
	m.d = dashboard{source: "archive", obsTime: time.Now()}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(nowModel)
	if m.scroll != 1 {
		t.Fatalf("down did not scroll: %d", m.scroll)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(nowModel)
	if m.scroll != 0 {
		t.Fatal("resize did not reset scroll position")
	}
	assertFrameBounds(t, m.View(), 100, 40)
}

func TestExploreFramesFitAtEveryStage(t *testing.T) {
	st := seedArchive(t)
	for _, view := range []exploreView{viewDay, viewWeek, viewMonth, viewYear, viewRecords} {
		data, err := loadExplore(context.Background(), st, view, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, size := range [][2]int{{0, 0}, {1, 1}, {20, 3}, {67, 24}, {68, 24}, {80, 10}, {80, 24}, {120, 40}} {
			for _, stage := range []string{"splash", "loading", "error", "data"} {
				m := newExploreModel(st, store.Coverage{})
				m.view, m.data = view, data
				m.width, m.height = size[0], size[1]
				m.haveOne = stage != "splash"
				m.loading = stage == "splash" || stage == "loading"
				if stage == "error" {
					m.err = errors.New(strings.Repeat("archive unavailable ", 20))
				}
				assertFrameBounds(t, m.View(), size[0], size[1])
			}
		}
	}
}

func TestExploreRefreshClearsStaleViewAndRejectsOldResponse(t *testing.T) {
	m := newExploreModel(nil, store.Coverage{})
	m.haveOne, m.loading, m.scroll = true, false, 7
	m.data.label = "OLD PERIOD"
	m.err = errors.New("old failure")
	next, cmd := m.switchView(viewMonth)
	m = next.(exploreModel)
	if cmd == nil || !m.loading || m.err != nil || m.scroll != 0 || m.data.label == "OLD PERIOD" {
		t.Fatalf("new view kept stale state: %+v", m)
	}
	if body := m.bodyFor(); !strings.Contains(body, "Reading this view") || strings.Contains(body, "old failure") {
		t.Fatalf("new view did not show a loading state: %q", body)
	}
	label := m.data.label
	next, _ = m.Update(exploreDataMsg{gen: m.gen - 1, data: exploreData{label: "STALE RESPONSE"}})
	m = next.(exploreModel)
	if m.data.label != label || !m.loading {
		t.Fatal("stale asynchronous response replaced the pending view")
	}
	next, _ = m.Update(exploreDataMsg{gen: m.gen, data: exploreData{label: "NEW PERIOD"}})
	m = next.(exploreModel)
	if m.loading || m.data.label != "NEW PERIOD" {
		t.Fatal("current asynchronous response was not accepted")
	}
}

func TestExploreRetryDoesNotOverlapAndPreservesBindings(t *testing.T) {
	m := newExploreModel(nil, store.Coverage{})
	m.haveOne, m.loading = true, false
	m.err = errors.New("retryable failure")
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(exploreModel)
	if cmd == nil || !m.loading || m.err != nil {
		t.Fatal("enter did not retry a failed load")
	}
	gen := m.gen
	next, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(exploreModel)
	if cmd != nil || m.gen != gen {
		t.Fatal("enter started a duplicate fetch")
	}
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(exploreModel)
	if m.view != viewRecords {
		t.Fatal("r no longer selects records")
	}
	m.view, m.offset = viewDay, 2
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyHome})
	if next.(exploreModel).offset != 0 {
		t.Fatal("Home no longer returns to the latest period")
	}
}
