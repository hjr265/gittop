package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel returns a model on the given tab with nothing left to load.
func newTestModel(tab int) model {
	return model{
		activeTab: tab,
		pages: []Page{
			newSummaryPage(),
			newActivityPage(),
			newContributorsPage(),
			newBranchesPage(),
			newHealthPage(),
			newReleasesPage(),
			newCommitsPage(nil),
		},
		commitsWithFiles: []CommitInfo{},
		branchesLoaded:   true,
		healthLoaded:     true,
	}
}

// press sends each rune of keys to the model as a key press.
func press(t *testing.T, m model, keys string) model {
	t.Helper()
	for _, r := range keys {
		m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func send(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	updated, cmd := m.Update(msg)
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatalf("%v quit the program", msg)
		}
	}
	return updated.(model)
}

func TestFileFilterInputTakesEveryKey(t *testing.T) {
	m := newTestModel(TabFiles)
	hp := m.pages[TabFiles].(*healthPage)

	// Each of these is a global shortcut outside the input.
	m = press(t, m, "f")
	m = press(t, m, "or-q/1+")
	if hp.pathInput != "or-q/1+" {
		t.Errorf("path input = %q, want %q", hp.pathInput, "or-q/1+")
	}
	if m.optionsOpen || m.loading || m.filterActive || m.activeTab != TabFiles {
		t.Errorf("typing in the file filter triggered a global shortcut: options=%v loading=%v filter=%v tab=%d",
			m.optionsOpen, m.loading, m.filterActive, m.activeTab)
	}

	// Esc cancels the edit and leaves no pattern behind.
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if hp.editing || hp.pathPattern != "" {
		t.Errorf("after esc: editing=%v pattern=%q, want false and empty", hp.editing, hp.pathPattern)
	}

	// Enter applies a pattern; esc then clears it.
	m = press(t, m, "f*.go")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if hp.editing || hp.pathPattern != "*.go" {
		t.Errorf("after enter: editing=%v pattern=%q, want false and %q", hp.editing, hp.pathPattern, "*.go")
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if hp.pathPattern != "" {
		t.Errorf("after esc: pattern=%q, want empty", hp.pathPattern)
	}

	// Outside the input the shortcuts work again.
	m = press(t, m, "o")
	if !m.optionsOpen {
		t.Errorf("o did not open the options menu outside the file filter")
	}
}
