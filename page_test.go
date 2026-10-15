package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestPageStep(t *testing.T) {
	tests := []struct {
		key  string
		rows int
		want int
	}{
		{"pgdown", 20, 20},
		{"pgup", 20, -20},
		{"ctrl+d", 20, 10},
		{"ctrl+u", 20, -10},
		// Always move at least one row, even before the first render.
		{"pgdown", 0, 1},
		{"ctrl+u", 1, -1},
		{"j", 20, 0},
	}
	for _, tt := range tests {
		got := pageStep(tt.key, tt.rows)
		if got != tt.want {
			t.Errorf("pageStep(%q, %d) = %d, want %d", tt.key, tt.rows, got, tt.want)
		}
	}
}

func TestCommitsPagePaging(t *testing.T) {
	p := newCommitsPage(nil)
	p.Update(commitsDataMsg{commits: make([]CommitInfo, 100)})
	p.View(80, 23) // 20 visible rows

	tests := []struct {
		key  tea.KeyMsg
		want int
	}{
		{tea.KeyMsg{Type: tea.KeyPgDown}, 20},
		{tea.KeyMsg{Type: tea.KeyCtrlD}, 30},
		{tea.KeyMsg{Type: tea.KeyPgUp}, 10},
		{tea.KeyMsg{Type: tea.KeyCtrlU}, 0},
		{tea.KeyMsg{Type: tea.KeyPgUp}, 0},
		{tea.KeyMsg{Type: tea.KeyEnd}, 99},
		{tea.KeyMsg{Type: tea.KeyPgDown}, 99},
		{tea.KeyMsg{Type: tea.KeyHome}, 0},
	}
	for _, tt := range tests {
		p.Update(tt.key)
		p.View(80, 23)
		if p.cursor != tt.want {
			t.Errorf("after %q cursor = %d, want %d", tt.key.String(), p.cursor, tt.want)
		}
	}
}

func TestWheelScrollsCommits(t *testing.T) {
	p := newCommitsPage(nil)
	p.Update(commitsDataMsg{commits: make([]CommitInfo, 100)})
	p.View(80, 23)

	down := tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	up := tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}

	p.Update(down)
	p.Update(down)
	if p.cursor != 2*wheelRows {
		t.Errorf("after two wheel downs cursor = %d, want %d", p.cursor, 2*wheelRows)
	}
	p.Update(up)
	p.Update(up)
	p.Update(up)
	if p.cursor != 0 {
		t.Errorf("after three wheel ups cursor = %d, want 0", p.cursor)
	}
}

func TestTabAt(t *testing.T) {
	bar := model{width: 200}.viewTabBar()
	for i, name := range tabNames {
		x := lipgloss.Width(bar[:strings.Index(bar, name)])
		if got := tabAt(x); got != i {
			t.Errorf("tabAt(%d) = %d, want %d (%s)", x, got, i, name)
		}
	}
	if got := tabAt(lipgloss.Width(strings.TrimRight(bar, " ")) + 5); got != -1 {
		t.Errorf("tabAt past the last tab = %d, want -1", got)
	}
}

func TestBindingAt(t *testing.T) {
	m := model{width: 200, activeTab: TabActivity, pages: make([]Page, len(tabNames))}
	bar := m.viewBottomBar()
	bindings := m.bottomBindings()
	for _, b := range bindings {
		x := lipgloss.Width(bar[:strings.Index(bar, b.desc)])
		got, ok := bindingAt(bindings, x)
		if !ok || got.desc != b.desc {
			t.Errorf("bindingAt(%d) = %q, %v, want %q", x, got.desc, ok, b.desc)
		}
	}
	if _, ok := bindingAt(bindings, lipgloss.Width(strings.TrimRight(bar, " "))+5); ok {
		t.Errorf("bindingAt past the last binding found a binding")
	}
}

func TestOptionRowAt(t *testing.T) {
	labels := []string{"Color theme", "Truecolor", "Graph symbol", "Mouse"}
	sizes := []struct{ width, height int }{{100, 27}, {101, 28}, {80, 21}, {120, 40}}
	for _, size := range sizes {
		lines := strings.Split(model{}.viewOptions(size.width, size.height), "\n")
		for want, label := range labels {
			found := false
			for y, line := range lines {
				idx := strings.Index(line, label)
				if idx < 0 {
					continue
				}
				found = true
				x := lipgloss.Width(line[:idx])
				row, inside := optionRowAt(x, y, size.width, size.height)
				if !inside || row != want {
					t.Errorf("%dx%d: optionRowAt(%d, %d) = %d, %v, want %d, true", size.width, size.height, x, y, row, inside, want)
				}
			}
			if !found {
				t.Errorf("%dx%d: option %q not rendered", size.width, size.height, label)
			}
		}
		if _, inside := optionRowAt(0, 0, size.width, size.height); inside {
			t.Errorf("%dx%d: optionRowAt(0, 0) is inside the menu", size.width, size.height)
		}
	}
}
