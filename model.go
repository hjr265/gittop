package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
)

const (
	chromeTopHeight    = 2 // top bar + tab bar
	chromeBottomHeight = 1 // keybinding bar or filter input
)

type model struct {
	repo     *git.Repository
	repoPath string
	width    int
	height   int
	err      error
	loading  bool

	activeTab int
	pages     []Page

	branch string

	// Commit data.
	commits         []CommitInfo // all commits (without files)
	commitsWithFiles []CommitInfo // all commits (with files, lazily populated)
	stats           []DayStat   // unfiltered daily stats

	// Filter state.
	filterInput   textinput.Model
	filterActive  bool // true when typing in filter input
	filterQuery   string
	filterExpr    FilterExpr
	filterErr     error
	filteredStats []DayStat
	filtering     bool // true while re-scanning with files

	// Global date range.
	rangeIdx int

	// Health data.
	healthLoaded  bool
	healthLoading bool
}

type commitsMsg struct {
	commits []CommitInfo
	branch  string
	err     error
}

// commitsWithFilesMsg is sent when a re-scan with file info completes.
type commitsWithFilesMsg struct {
	commits []CommitInfo
	err     error
}

type statsMsg struct {
	stats []DayStat
}

type commitsDataMsg struct {
	commits  []CommitInfo
	filtered bool // true when a filter is active
}

func newModel(repo *git.Repository, path string) model {
	ti := textinput.New()
	ti.Placeholder = `author:"name" and ("fix" or "bug") and path:*.go`
	ti.CharLimit = 256
	ti.Width = 80

	m := model{
		repo:        repo,
		repoPath:    path,
		loading:     true,
		activeTab:   TabSummary,
		rangeIdx:    defaultRangeIdx,
		filterInput: ti,
	}
	m.pages = []Page{
		newSummaryPage(),
		newActivityPage(),
		newContributorsPage(),
		newBranchesPage(),
		newFilesPage(),
		newLogPage(),
		newHealthPage(),
	}
	return m
}

func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		commits, err := CollectCommits(m.repo, false)
		branch := ""
		if ref, e := m.repo.Head(); e == nil {
			if ref.Name().IsBranch() {
				branch = ref.Name().Short()
			} else {
				branch = ref.Hash().String()[:8]
			}
		}
		return commitsMsg{commits: commits, branch: branch, err: err}
	}
}

func (m *model) applyFilter() {
	m.filterErr = nil
	if m.filterQuery == "" {
		m.filterExpr = nil
		m.filteredStats = m.stats
		m.propagateStats()
		return
	}
	expr, err := ParseFilter(m.filterQuery)
	if err != nil {
		m.filterErr = err
		return
	}
	m.filterExpr = expr
	m.recomputeFilteredStats()
}

func (m *model) recomputeFilteredStats() {
	if m.filterExpr == nil {
		m.filteredStats = m.stats
		m.propagateStats()
		return
	}

	// If the filter needs file info and we don't have it yet, trigger a re-scan.
	source := m.commits
	if FilterNeedsFiles(m.filterExpr) {
		if m.commitsWithFiles != nil {
			source = m.commitsWithFiles
		}
		// If we haven't loaded files yet, filteredStats will be approximate
		// (path filters won't match). The re-scan command is triggered in Update.
	}

	filtered := FilterCommits(source, m.filterExpr)
	m.filteredStats = CommitsToDailyStats(filtered)
	m.propagateStats()
}

func (m *model) rangeCutoff() time.Time {
	r := rangePresets[m.rangeIdx]
	if r.days == 0 {
		return time.Time{} // no cutoff
	}
	return truncateToDay(time.Now()).AddDate(0, 0, -r.days)
}

func (m *model) propagateStats() {
	cutoff := m.rangeCutoff()
	stats := applyRangeCutoff(m.filteredStats, cutoff)
	sm := statsMsg{stats: stats}
	for i, p := range m.pages {
		updated, _ := p.Update(sm)
		m.pages[i] = updated
	}

	// Propagate filtered commits for pages that need them.
	// Prefer commitsWithFiles when available (needed by Health tab).
	source := m.commits
	if m.commitsWithFiles != nil {
		source = m.commitsWithFiles
	}
	if m.filterExpr != nil {
		source = FilterCommits(source, m.filterExpr)
	}
	source = applyRangeCutoffCommits(source, cutoff)
	cdMsg := commitsDataMsg{commits: source, filtered: m.filterExpr != nil}
	for i, p := range m.pages {
		updated, _ := p.Update(cdMsg)
		m.pages[i] = updated
	}
}

func applyRangeCutoff(stats []DayStat, cutoff time.Time) []DayStat {
	if cutoff.IsZero() || len(stats) == 0 {
		return stats
	}
	for i, s := range stats {
		if !s.Date.Before(cutoff) {
			return stats[i:]
		}
	}
	return nil
}

func applyRangeCutoffCommits(commits []CommitInfo, cutoff time.Time) []CommitInfo {
	if cutoff.IsZero() || len(commits) == 0 {
		return commits
	}
	var result []CommitInfo
	for i := range commits {
		if !commits[i].Date.Before(cutoff) {
			result = append(result, commits[i])
		}
	}
	return result
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.filterInput.Width = m.width - 20
		if m.filterInput.Width < 20 {
			m.filterInput.Width = 20
		}

	case commitsMsg:
		m.commits = msg.commits
		m.branch = msg.branch
		m.err = msg.err
		m.loading = false
		m.stats = CommitsToDailyStats(m.commits)
		m.filteredStats = m.stats
		m.propagateStats()
		return m, nil

	case commitsWithFilesMsg:
		m.filtering = false
		if msg.err == nil {
			m.commitsWithFiles = msg.commits
			m.recomputeFilteredStats()
		}
		return m, nil

	case healthTreeMsg:
		m.healthLoading = false
		m.healthLoaded = true
		// Send tree data to the health page.
		for i, p := range m.pages {
			updated, _ := p.Update(msg)
			m.pages[i] = updated
		}
		return m, nil

	case tea.KeyMsg:
		// Filter input mode.
		if m.filterActive {
			switch msg.String() {
			case "enter":
				m.filterQuery = m.filterInput.Value()
				m.filterActive = false
				m.filterInput.Blur()
				m.applyFilter()

				// If filter needs files and we don't have them, trigger re-scan.
				if m.filterExpr != nil && FilterNeedsFiles(m.filterExpr) && m.commitsWithFiles == nil && !m.filtering {
					m.filtering = true
					return m, func() tea.Msg {
						commits, err := CollectCommits(m.repo, true)
						return commitsWithFilesMsg{commits: commits, err: err}
					}
				}
				return m, nil
			case "esc":
				m.filterActive = false
				m.filterInput.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.filterInput, cmd = m.filterInput.Update(msg)
				return m, cmd
			}
		}

		// Normal mode.
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.filterActive = true
			m.filterInput.Focus()
			return m, m.filterInput.Cursor.BlinkCmd()
		case "esc":
			// Clear filter.
			if m.filterQuery != "" {
				m.filterQuery = ""
				m.filterExpr = nil
				m.filterErr = nil
				m.filterInput.SetValue("")
				m.filteredStats = m.stats
				m.propagateStats()
				return m, nil
			}
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
		case "7":
			m.activeTab = TabHealth
		case "+", "=":
			if m.rangeIdx < len(rangePresets)-1 {
				m.rangeIdx++
				m.propagateStats()
			}
			return m, nil
		case "-", "_":
			if m.rangeIdx > 0 {
				m.rangeIdx--
				m.propagateStats()
			}
			return m, nil
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

	// Trigger health data loading when Health tab is active.
	if m.activeTab == TabHealth && !m.healthLoaded && !m.healthLoading {
		m.healthLoading = true
		repo := m.repo
		cmds := []tea.Cmd{
			func() tea.Msg {
				lineCounts, err := CollectFileLineCounts(repo)
				return healthTreeMsg{lineCounts: lineCounts, err: err}
			},
		}
		// Also load commits with files if not already loaded.
		if m.commitsWithFiles == nil && !m.filtering {
			m.filtering = true
			cmds = append(cmds, func() tea.Msg {
				commits, err := CollectCommits(repo, true)
				return commitsWithFilesMsg{commits: commits, err: err}
			})
		}
		return m, tea.Batch(cmds...)
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

	// Bottom bar or filter input.
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

	// Show filter indicator.
	filterIndicator := ""
	if m.filterQuery != "" {
		filterStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("214")).
			Bold(true)
		filterIndicator = dimOnBar.Render("  ") + filterStyle.Render("filter: "+m.filterQuery)
	}
	if m.filtering {
		filterIndicator += dimOnBar.Render(" (scanning files...)")
	}

	// Range indicator.
	rangeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("82")).
		Bold(true)
	rangeIndicator := dimOnBar.Render("  ") + rangeStyle.Render(rangePresets[m.rangeIdx].label)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(filterIndicator) - lipgloss.Width(rangeIndicator) - 1
	if gap < 0 {
		gap = 0
	}
	filler := lipgloss.NewStyle().Background(lipgloss.Color("236")).Render(strings.Repeat(" ", gap))

	return left + filterIndicator + filler + rangeIndicator + dimOnBar.Render(" ")
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

	// Filter input mode.
	if m.filterActive {
		promptStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("214")).
			Bold(true)

		inputView := m.filterInput.View()
		content := promptStyle.Render(" / ") + inputView

		gap := m.width - lipgloss.Width(content)
		if gap < 0 {
			gap = 0
		}
		filler := barStyle.Render(strings.Repeat(" ", gap))
		return content + filler
	}

	// Normal bottom bar.
	bindings := []struct{ key, desc string }{
		{"1-7", "pages"},
		{"tab", "next"},
		{"+/-", "range"},
		{"/", "filter"},
		{"q", "quit"},
	}

	if m.filterQuery != "" {
		bindings = append(bindings, struct{ key, desc string }{"esc", "clear filter"})
	}

	// Page-specific bindings.
	if m.activeTab == TabSummary {
		bindings = append(bindings,
			struct{ key, desc string }{"d/w/m/y", "granularity"},
		)
	}
	if m.activeTab == TabActivity {
		bindings = append(bindings,
			struct{ key, desc string }{"v", "cycle view"},
		)
	}
	if m.activeTab == TabHealth {
		bindings = append(bindings,
			struct{ key, desc string }{"v", "cycle view"},
			struct{ key, desc string }{"f", "file filter"},
		)
	}

	var parts []string
	for _, bind := range bindings {
		parts = append(parts, " "+keyStyle.Render(bind.key)+" "+barStyle.Render(bind.desc))
	}

	content := barStyle.Render("") + strings.Join(parts, barStyle.Render("  "))

	// Show filter error if any.
	if m.filterErr != nil {
		errStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("196")).
			Bold(true)
		content += errStyle.Render("  " + m.filterErr.Error())
	}

	gap := m.width - lipgloss.Width(content)
	if gap < 0 {
		gap = 0
	}
	filler := barStyle.Render(strings.Repeat(" ", gap))

	return content + filler
}
