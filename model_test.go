package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// writeTestConfig points the config file at a temporary directory and writes
// content to it. It also restores the color profile, which newModel sets
// globally, when the test ends.
func writeTestConfig(t *testing.T, content string) {
	t.Helper()
	profile := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir) // macOS ignores XDG_CONFIG_HOME
	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStartingRange(t *testing.T) {
	tests := []struct {
		name   string
		config string
		flag   string
		want   string
	}{
		{"built-in default", ``, "", "1y"},
		{"config", `default_range = "all"`, "", "all"},
		{"flag overrides config", `default_range = "all"`, "3m", "3m"},
		{"unknown config value", `default_range = "7y"`, "", "1y"},
	}
	for _, tt := range tests {
		writeTestConfig(t, tt.config)
		m := newModel(nil, ".", tt.flag)
		if got := rangePresets[m.rangeIdx].label; got != tt.want {
			t.Errorf("%s: starting range = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestToConfigKeepsUnmanagedSettings(t *testing.T) {
	writeTestConfig(t, `
theme = "default"
default_range = "2y"

[custom_theme.text]
accent = "#ff0000"
`)
	m := newModel(nil, ".", "")
	cfg := m.ToConfig()
	if cfg.DefaultRange != "2y" {
		t.Errorf("DefaultRange = %q, want %q", cfg.DefaultRange, "2y")
	}
	if cfg.CustomTheme == nil {
		t.Error("CustomTheme was dropped")
	}
	if names := availableThemeNames(m); !slices.Contains(names, "Custom") {
		t.Errorf("theme names %v do not offer Custom", names)
	}
}
