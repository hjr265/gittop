package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
