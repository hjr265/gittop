package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
)

const (
	chromeTopHeight    = 2 // top bar + tab bar
	chromeBottomHeight = 1 // keybinding bar
	maxFetchDays       = 365
)

type model struct {
	repo     *git.Repository
	repoPath string
	stats    []DayStat
	width    int
	height   int
	err      error
	loading  bool

	activeTab int
	pages     []Page

	branch    string
	fetchedAt time.Time
}

type statsMsg struct {
	stats  []DayStat
	branch string
	err    error
}

func newModel(repo *git.Repository, path string) model {
	m := model{
		repo:      repo,
		repoPath:  path,
		loading:   true,
		activeTab: TabSummary,
	}
	// Pages are initialized after stats are loaded.
	m.pages = []Page{
		newSummaryPage(),
		newActivityPage(),
		newContributorsPage(),
		newBranchesPage(),
		newFilesPage(),
		newLogPage(),
	}
	return m
}

func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		stats, err := CollectDailyStats(m.repo, maxFetchDays)
		branch := ""
		if ref, e := m.repo.Head(); e == nil {
			if ref.Name().IsBranch() {
				branch = ref.Name().Short()
			} else {
				branch = ref.Hash().String()[:8]
			}
		}
		return statsMsg{stats: stats, branch: branch, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case statsMsg:
		m.stats = msg.stats
		m.branch = msg.branch
		m.err = msg.err
		m.loading = false
		m.fetchedAt = time.Now()

		// Propagate stats to pages that need them.
		var cmds []tea.Cmd
		for i, p := range m.pages {
			updated, cmd := p.Update(msg)
			m.pages[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "1":
			m.activeTab = TabSummary
		case "2":
			m.activeTab = TabActivity
		case "3":
			m.activeTab = TabContributors
		case "4":
			m.activeTab = TabBranches
		case "5":
			m.activeTab = TabFiles
		case "6":
			m.activeTab = TabLog
		case "tab":
			m.activeTab = (m.activeTab + 1) % len(m.pages)
		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + len(m.pages)) % len(m.pages)
		default:
			// Delegate to active page.
			if m.activeTab >= 0 && m.activeTab < len(m.pages) {
				updated, cmd := m.pages[m.activeTab].Update(msg)
				m.pages[m.activeTab] = updated
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.loading {
		return "\n  Scanning repository..."
	}
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n", m.err)
	}

	var b strings.Builder

	// Top bar.
	b.WriteString(m.viewTopBar())
	b.WriteString("\n")

	// Tab bar.
	b.WriteString(m.viewTabBar())
	b.WriteString("\n")

	// Page content.
	pageHeight := m.height - chromeTopHeight - chromeBottomHeight
	if pageHeight < 1 {
		pageHeight = 1
	}
	content := m.pages[m.activeTab].View(m.width, pageHeight)

	// Pad or truncate content to fill the page area.
	lines := strings.Split(content, "\n")
	for len(lines) < pageHeight {
		lines = append(lines, "")
	}
	if len(lines) > pageHeight {
		lines = lines[:pageHeight]
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")

	// Bottom bar.
	b.WriteString(m.viewBottomBar())

	return b.String()
}

func (m model) viewTopBar() string {
	topBarStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Bold(true)

	branchStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("205"))

	dimOnBar := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("245"))

	left := topBarStyle.Render(" gittop") +
		dimOnBar.Render(": ") +
		topBarStyle.Render(m.repoPath) +
		dimOnBar.Render("  ") +
		branchStyle.Render(m.branch)

	right := dimOnBar.Render(m.fetchedAt.Format("15:04:05") + " ")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	filler := lipgloss.NewStyle().Background(lipgloss.Color("236")).Render(strings.Repeat(" ", gap))

	return left + filler + right
}

func (m model) viewTabBar() string {
	var parts []string

	for i, name := range tabNames {
		key := fmt.Sprintf("%d", i+1)
		if i == m.activeTab {
			tab := lipgloss.NewStyle().
				Background(lipgloss.Color("63")).
				Foreground(lipgloss.Color("255")).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("[%s] %s", key, name))
			parts = append(parts, tab)
		} else {
			tab := lipgloss.NewStyle().
				Background(lipgloss.Color("235")).
				Foreground(lipgloss.Color("245")).
				Padding(0, 1).
				Render(fmt.Sprintf(" %s  %s", key, name))
			parts = append(parts, tab)
		}
	}

	tabContent := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	// Fill remaining width.
	gap := m.width - lipgloss.Width(tabContent)
	if gap < 0 {
		gap = 0
	}
	filler := lipgloss.NewStyle().Background(lipgloss.Color("235")).Render(strings.Repeat(" ", gap))

	return tabContent + filler
}

func (m model) viewBottomBar() string {
	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("245"))

	keyStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("255")).
		Bold(true)

	bindings := []struct{ key, desc string }{
		{"1-6", "pages"},
		{"tab", "next"},
		{"q", "quit"},
	}

	var parts []string
	for _, bind := range bindings {
		parts = append(parts, " "+keyStyle.Render(bind.key)+" "+barStyle.Render(bind.desc))
	}

	content := barStyle.Render("") + strings.Join(parts, barStyle.Render("  "))

	gap := m.width - lipgloss.Width(content)
	if gap < 0 {
		gap = 0
	}
	filler := barStyle.Render(strings.Repeat(" ", gap))

	return content + filler
}
