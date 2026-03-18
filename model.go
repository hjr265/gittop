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

	branch  string
	mailmap *Mailmap

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

	// Branch data.
	branchesLoaded  bool
	branchesLoading bool

	// Tag/release data.
	tags        []TagInfo
	commitsSince int // commits since latest tag (unfiltered)
	tagsLoaded  bool
	tagsLoading bool

	// Health data.
	healthLoaded  bool
	healthLoading bool

	// Options menu.
	optionsOpen   bool
	optionsCursor int
	graphSymbol   GraphSymbol
}

type commitsMsg struct {
	commits []CommitInfo
	branch  string
	err     error
	mailmap *Mailmap
}

// commitsWithFilesMsg is sent when a re-scan with file info completes.
type commitsWithFilesMsg struct {
	commits []CommitInfo
	err     error
}

type statsMsg struct {
	stats []DayStat
}

type tagsDataMsg struct {
	tags          []TagInfo
	commitsSince  int // commits since latest tag
	err           error
}

type commitsDataMsg struct {
	commits  []CommitInfo
	filtered bool // true when a filter is active
}

func newModel(repo *git.Repository, path string) model {
	ti := textinput.New()
	ti.Placeholder = `author:"name" and ("fix" or "bug") and path:*.go and branch:main`
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
		newHealthPage(),
		newReleasesPage(),
		newCommitsPage(repo),
	}
	return m
}

func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		mm := LoadMailmap(m.repo)
		commits, err := CollectCommits(m.repo, false, mm)
		branch := ""
		if ref, e := m.repo.Head(); e == nil {
			if ref.Name().IsBranch() {
				branch = ref.Name().Short()
			} else {
				branch = ref.Hash().String()[:8]
			}
		}
		return commitsMsg{commits: commits, branch: branch, err: err, mailmap: mm}
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
	if FilterNeedsBranches(m.filterExpr) {
		PopulateBranchHashes(m.filterExpr, m.repo)
	}
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

	// Re-propagate tags with range cutoff and filter.
	if m.tagsLoaded {
		m.propagateTags(source)
	}
}

func (m *model) propagateTags(filteredCommits []CommitInfo) {
	cutoff := m.rangeCutoff()
	filtered := applyRangeCutoffTags(m.tags, cutoff)

	// If a filter is active, only keep tags whose commit is in the filtered set.
	if m.filterExpr != nil && len(filteredCommits) > 0 {
		hashes := make(map[string]bool, len(filteredCommits))
		for _, c := range filteredCommits {
			hashes[c.Hash] = true
		}
		var matched []TagInfo
		for _, t := range filtered {
			if hashes[t.CommitHash] {
				matched = append(matched, t)
			}
		}
		filtered = matched
	}

	msg := tagsDataMsg{tags: filtered, commitsSince: m.commitsSince}
	for i, p := range m.pages {
		updated, _ := p.Update(msg)
		m.pages[i] = updated
	}
}

func applyRangeCutoffTags(tags []TagInfo, cutoff time.Time) []TagInfo {
	if cutoff.IsZero() || len(tags) == 0 {
		return tags
	}
	var result []TagInfo
	for _, t := range tags {
		if !t.Date.Before(cutoff) {
			result = append(result, t)
		}
	}
	return result
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
		m.mailmap = msg.mailmap
		m.loading = false
		m.stats = CommitsToDailyStats(m.commits)
		m.filteredStats = m.stats
		m.propagateStats()
		// Start loading tags in background.
		if !m.tagsLoaded && !m.tagsLoading {
			m.tagsLoading = true
			repo := m.repo
			return m, func() tea.Msg {
				tags, err := CollectTags(repo)
				commitsSince := 0
				if err == nil && len(tags) > 0 && tags[0].CommitHash != "" {
					commitsSince = CountCommitsSince(repo, tags[0].CommitHash)
				}
				return tagsDataMsg{tags: tags, commitsSince: commitsSince, err: err}
			}
		}
		return m, nil

	case commitDiffMsg:
		// Route diff result to the commits page.
		updated, _ := m.pages[TabCommits].Update(msg)
		m.pages[TabCommits] = updated
		return m, nil

	case commitsWithFilesMsg:
		m.filtering = false
		if msg.err == nil {
			m.commitsWithFiles = msg.commits
			m.recomputeFilteredStats()
		}
		return m, nil

	case branchesDataMsg:
		m.branchesLoading = false
		m.branchesLoaded = true
		for i, p := range m.pages {
			updated, _ := p.Update(msg)
			m.pages[i] = updated
		}
		return m, nil

	case tagsDataMsg:
		m.tagsLoading = false
		m.tagsLoaded = true
		m.tags = msg.tags
		m.commitsSince = msg.commitsSince
		// Build filtered commits for tag filtering.
		cutoff := m.rangeCutoff()
		source := m.commits
		if m.commitsWithFiles != nil {
			source = m.commitsWithFiles
		}
		if m.filterExpr != nil {
			source = FilterCommits(source, m.filterExpr)
		}
		source = applyRangeCutoffCommits(source, cutoff)
		m.propagateTags(source)
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
		// Options menu mode.
		if m.optionsOpen {
			switch msg.String() {
			case "esc", "o", "q":
				m.optionsOpen = false
			case "up", "k":
				if m.optionsCursor > 0 {
					m.optionsCursor--
				}
			case "down", "j":
				if m.optionsCursor < 2 {
					m.optionsCursor++
				}
			case "left", "right", "h", "l", "enter":
				// Cycle graph symbol (only option that has multiple values).
				if m.optionsCursor == 2 {
					if m.graphSymbol == GraphBraille {
						m.graphSymbol = GraphBlock
					} else {
						m.graphSymbol = GraphBraille
					}
					msg := graphSymbolMsg{symbol: m.graphSymbol}
					for i, p := range m.pages {
						updated, _ := p.Update(msg)
						m.pages[i] = updated
					}
				}
			}
			return m, nil
		}

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
					mm := m.mailmap
					return m, func() tea.Msg {
						commits, err := CollectCommits(m.repo, true, mm)
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

		// Delegate to commits page when it's in diff view or search mode.
		if m.activeTab == TabCommits {
			if cp, ok := m.pages[TabCommits].(*commitsPage); ok && (cp.showDiff || cp.searching) {
				updated, cmd := m.pages[TabCommits].Update(msg)
				m.pages[TabCommits] = updated
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
			// On commits tab with active search, clear the search first.
			if m.activeTab == TabCommits {
				if cp, ok := m.pages[TabCommits].(*commitsPage); ok && cp.searchQuery != "" {
					updated, cmd := m.pages[TabCommits].Update(msg)
					m.pages[TabCommits] = updated
					return m, cmd
				}
			}
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
			m.activeTab = TabReleases
		case "7":
			m.activeTab = TabCommits
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
		case "r":
			// Manual refresh: re-scan the repository.
			m.loading = true
			m.commits = nil
			m.commitsWithFiles = nil
			m.stats = nil
			m.filteredStats = nil
			m.tags = nil
			m.tagsLoaded = false
			m.tagsLoading = false
			m.branchesLoaded = false
			m.branchesLoading = false
			m.healthLoaded = false
			m.healthLoading = false
			m.filtering = false
			m.commitsSince = 0
			repo := m.repo
			return m, func() tea.Msg {
				mm := LoadMailmap(repo)
				commits, err := CollectCommits(repo, false, mm)
				branch := ""
				if ref, e := repo.Head(); e == nil {
					if ref.Name().IsBranch() {
						branch = ref.Name().Short()
					} else {
						branch = ref.Hash().String()[:8]
					}
				}
				return commitsMsg{commits: commits, branch: branch, err: err, mailmap: mm}
			}
		case "o":
			m.optionsOpen = true
			m.optionsCursor = 0
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

	// Trigger branch data loading when Branches tab is active.
	if m.activeTab == TabBranches && !m.branchesLoaded && !m.branchesLoading {
		m.branchesLoading = true
		repo := m.repo
		mm := m.mailmap
		return m, func() tea.Msg {
			branches, err := CollectBranches(repo, mm)
			return branchesDataMsg{branches: branches, err: err}
		}
	}

	// Trigger file data loading for Contributors ownership view.
	if m.activeTab == TabContributors && m.commitsWithFiles == nil && !m.filtering {
		m.filtering = true
		repo := m.repo
		mm := m.mailmap
		return m, func() tea.Msg {
			commits, err := CollectCommits(repo, true, mm)
			return commitsWithFilesMsg{commits: commits, err: err}
		}
	}

	// Trigger health data loading when Health tab is active.
	if m.activeTab == TabFiles && !m.healthLoaded && !m.healthLoading {
		m.healthLoading = true
		repo := m.repo
		mm := m.mailmap
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
				commits, err := CollectCommits(repo, true, mm)
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

	var content string
	if m.optionsOpen {
		content = m.viewOptions(m.width, pageHeight)
	} else {
		content = m.pages[m.activeTab].View(m.width, pageHeight)
	}

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
		{"o", "options"},
		{"r", "refresh"},
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
	if m.activeTab == TabBranches {
		bindings = append(bindings,
			struct{ key, desc string }{"s/S", "sort/order"},
			struct{ key, desc string }{"j/k", "scroll"},
		)
	}
	if m.activeTab == TabActivity {
		bindings = append(bindings,
			struct{ key, desc string }{"v", "cycle view"},
		)
	}
	if m.activeTab == TabContributors {
		bindings = append(bindings,
			struct{ key, desc string }{"v", "cycle view"},
			struct{ key, desc string }{"j/k", "scroll"},
		)
	}
	if m.activeTab == TabReleases {
		bindings = append(bindings,
			struct{ key, desc string }{"v", "cycle view"},
			struct{ key, desc string }{"j/k", "scroll"},
		)
	}
	if m.activeTab == TabCommits {
		if cp, ok := m.pages[TabCommits].(*commitsPage); ok && cp.showDiff {
			bindings = append(bindings,
				struct{ key, desc string }{"esc", "back"},
				struct{ key, desc string }{"j/k", "scroll"},
				struct{ key, desc string }{"g/G", "top/bottom"},
			)
		} else if cp, ok := m.pages[TabCommits].(*commitsPage); ok && cp.searching {
			bindings = append(bindings,
				struct{ key, desc string }{"enter", "search"},
				struct{ key, desc string }{"esc", "cancel"},
			)
		} else {
			bindings = append(bindings,
				struct{ key, desc string }{"enter", "open diff"},
				struct{ key, desc string }{"s", "search"},
				struct{ key, desc string }{"j/k", "scroll"},
				struct{ key, desc string }{"g/G", "top/bottom"},
			)
			if cp, ok := m.pages[TabCommits].(*commitsPage); ok && cp.searchQuery != "" {
				bindings = append(bindings, struct{ key, desc string }{"esc", "clear search"})
			}
		}
	}
	if m.activeTab == TabFiles {
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

func (m model) viewOptions(width, height int) string {
	type option struct {
		label string
		value string
	}
	graphSymbolName := "Braille"
	if m.graphSymbol == GraphBlock {
		graphSymbolName = "Block"
	}
	options := []option{
		{"Color theme", "Default"},
		{"Truecolor", "True"},
		{"Graph symbol", graphSymbolName},
	}

	innerWidth := 36

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Background(lipgloss.Color("235"))

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true).
		Background(lipgloss.Color("235")).
		Width(innerWidth)

	normalLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("235"))

	normalValue := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("235"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Bold(true).
		Background(lipgloss.Color("63"))

	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("235")).
		Width(innerWidth)

	bgStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("235"))

	var rows []string
	rows = append(rows, titleStyle.Render(" Options"))
	rows = append(rows, bgStyle.Render(strings.Repeat(" ", innerWidth)))

	for i, opt := range options {
		valueStr := "‹ " + opt.value + " ›"
		gap := innerWidth - len(opt.label) - len(valueStr) - 2 // -2 for leading/trailing space
		if gap < 1 {
			gap = 1
		}
		if i == m.optionsCursor {
			line := selectedStyle.Render(" " + opt.label + strings.Repeat(" ", gap) + valueStr + " ")
			rows = append(rows, line)
		} else {
			line := normalLabel.Render(" "+opt.label) +
				bgStyle.Render(strings.Repeat(" ", gap)) +
				normalValue.Render(valueStr+" ")
			rows = append(rows, line)
		}
	}

	rows = append(rows, bgStyle.Render(strings.Repeat(" ", innerWidth)))
	rows = append(rows, hintStyle.Render(" esc: close  j/k: nav  h/l: change"))

	content := strings.Join(rows, "\n")
	box := borderStyle.Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
