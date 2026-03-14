package main

import (
	"bytes"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// commitDiffMsg carries the result of computing a commit diff.
type commitDiffMsg struct {
	hash    string
	diff    string
	author  string
	date    string
	err     error
}

type commitsPage struct {
	repo    *git.Repository
	commits []CommitInfo
	offset  int // scroll offset (top of visible window)
	cursor  int // selected row index (absolute, within filtered/full list)

	// Search state.
	searching   bool   // true when typing search query
	searchInput string // current input while editing
	searchQuery string // active search query
	filtered    []int  // indices into commits matching searchQuery

	// Diff view state.
	showDiff    bool
	diffHash    string
	diff        string
	diffHeader  string
	diffOffset  int
	diffLoading bool
}

func newCommitsPage(repo *git.Repository) *commitsPage {
	return &commitsPage{repo: repo}
}

func (p *commitsPage) Init() tea.Cmd { return nil }

func (p *commitsPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case commitsDataMsg:
		p.commits = msg.commits
		p.refilter()
		if p.cursor >= p.visibleLen() {
			p.cursor = 0
			p.offset = 0
		}
	case commitDiffMsg:
		p.diffLoading = false
		if msg.err != nil {
			p.diff = fmt.Sprintf("Error: %v", msg.err)
		} else {
			p.diff = msg.diff
		}
		p.diffHash = msg.hash
		p.diffHeader = fmt.Sprintf("%s  %s  %s", msg.hash, msg.author, msg.date)
		p.diffOffset = 0
		p.showDiff = true
	case tea.KeyMsg:
		if p.showDiff {
			return p.updateDiffView(msg)
		}
		if p.searching {
			return p.updateSearch(msg)
		}
		return p.updateListView(msg)
	}
	return p, nil
}

// visibleLen returns the number of commits currently visible (filtered or all).
func (p *commitsPage) visibleLen() int {
	if p.searchQuery != "" {
		return len(p.filtered)
	}
	return len(p.commits)
}

// visibleCommit returns the commit at the given visible index.
func (p *commitsPage) visibleCommit(i int) CommitInfo {
	if p.searchQuery != "" {
		return p.commits[p.filtered[i]]
	}
	return p.commits[i]
}

// refilter recomputes the filtered index list based on the current search query.
func (p *commitsPage) refilter() {
	if p.searchQuery == "" {
		p.filtered = nil
		return
	}
	q := strings.ToLower(p.searchQuery)
	p.filtered = nil
	for i, c := range p.commits {
		if fuzzyMatch(q, c) {
			p.filtered = append(p.filtered, i)
		}
	}
}

func fuzzyMatch(query string, c CommitInfo) bool {
	// Split query into terms for fuzzy matching.
	terms := strings.Fields(query)
	for _, term := range terms {
		found := strings.Contains(strings.ToLower(c.Hash), term) ||
			strings.Contains(strings.ToLower(c.Author), term) ||
			strings.Contains(strings.ToLower(c.Email), term) ||
			strings.Contains(strings.ToLower(c.Message), term)
		if !found {
			return false
		}
	}
	return true
}

func (p *commitsPage) updateSearch(msg tea.KeyMsg) (Page, tea.Cmd) {
	switch msg.String() {
	case "enter":
		p.searchQuery = p.searchInput
		p.searching = false
		p.refilter()
		p.cursor = 0
		p.offset = 0
	case "esc":
		p.searching = false
		p.searchInput = p.searchQuery
	case "backspace":
		if len(p.searchInput) > 0 {
			p.searchInput = p.searchInput[:len(p.searchInput)-1]
		}
	default:
		if len(msg.String()) == 1 {
			p.searchInput += msg.String()
		}
	}
	return p, nil
}

func (p *commitsPage) updateListView(msg tea.KeyMsg) (Page, tea.Cmd) {
	n := p.visibleLen()
	switch msg.String() {
	case "j", "down":
		if p.cursor < n-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "g":
		p.cursor = 0
		p.offset = 0
	case "G":
		if n > 0 {
			p.cursor = n - 1
		}
	case "s":
		p.searching = true
		p.searchInput = p.searchQuery
	case "esc":
		if p.searchQuery != "" {
			p.searchQuery = ""
			p.searchInput = ""
			p.filtered = nil
			p.cursor = 0
			p.offset = 0
		}
	case "enter":
		if p.cursor >= 0 && p.cursor < n && p.repo != nil {
			c := p.visibleCommit(p.cursor)
			p.diffLoading = true
			repo := p.repo
			return p, func() tea.Msg {
				return computeDiff(repo, c)
			}
		}
	}
	return p, nil
}

func (p *commitsPage) updateDiffView(msg tea.KeyMsg) (Page, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "enter":
		p.showDiff = false
		p.diff = ""
		p.diffOffset = 0
	case "j", "down":
		p.diffOffset++
	case "k", "up":
		if p.diffOffset > 0 {
			p.diffOffset--
		}
	case "g":
		p.diffOffset = 0
	case "G":
		p.diffOffset = 999999
	}
	return p, nil
}

func computeDiff(repo *git.Repository, ci CommitInfo) commitDiffMsg {
	hash := ci.Hash
	if len(hash) > 7 {
		hash = hash[:7]
	}
	result := commitDiffMsg{
		hash:   hash,
		author: ci.Author,
		date:   ci.Date.Format("2006-01-02"),
	}

	commit, err := repo.CommitObject(plumbing.NewHash(ci.Hash))
	if err != nil {
		result.err = err
		return result
	}

	var parent *object.Commit
	if commit.NumParents() > 0 {
		p, err := commit.Parents().Next()
		if err == nil {
			parent = p
		}
	}

	var patch *object.Patch
	if parent != nil {
		patch, err = parent.Patch(commit)
	} else {
		patch, err = commit.Patch(nil)
	}
	if err != nil {
		result.err = err
		return result
	}

	var buf bytes.Buffer
	if err := patch.Encode(&buf); err != nil {
		result.err = err
		return result
	}

	result.diff = buf.String()
	return result
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func (p *commitsPage) View(width, height int) string {
	if p.showDiff {
		return p.viewDiff(width, height)
	}
	if p.diffLoading {
		return "\n  Loading diff..."
	}
	return p.viewList(width, height)
}

func (p *commitsPage) viewList(width, height int) string {
	n := p.visibleLen()

	if len(p.commits) == 0 {
		return "\n  No commits in selected range."
	}

	var b strings.Builder
	b.WriteString("\n")

	hashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	authorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("237"))

	// Header with count and search indicator.
	header := fmt.Sprintf("%d commits", len(p.commits))
	if p.searching {
		filterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
		header += "    search: " + filterStyle.Render(p.searchInput+"_")
	} else if p.searchQuery != "" {
		filterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
		header = fmt.Sprintf("%d/%d commits    search: ", n, len(p.commits)) + filterStyle.Render(p.searchQuery)
	}
	b.WriteString(fmt.Sprintf("  %s\n\n", dimStyle.Render(header)))

	if n == 0 && p.searchQuery != "" {
		b.WriteString("  No matching commits.")
		return b.String()
	}

	visibleRows := height - 3
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Ensure cursor is visible.
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+visibleRows {
		p.offset = p.cursor - visibleRows + 1
	}

	maxOffset := n - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}

	end := p.offset + visibleRows
	if end > n {
		end = n
	}

	authorWidth := 16
	dateWidth := 10
	hashWidth := 7
	chrome := 2 + hashWidth + 1 + authorWidth + 1 + dateWidth + 1
	msgWidth := width - chrome
	if msgWidth < 10 {
		msgWidth = 10
	}

	for i := p.offset; i < end; i++ {
		c := p.visibleCommit(i)

		hash := c.Hash
		if len(hash) > hashWidth {
			hash = hash[:hashWidth]
		}

		author := c.Author
		if len(author) > authorWidth {
			author = author[:authorWidth-1] + "…"
		}

		date := c.Date.Format("2006-01-02")
		if len(date) > dateWidth {
			date = date[:dateWidth]
		}

		message := firstLine(c.Message)
		if len(message) > msgWidth {
			message = message[:msgWidth-1] + "…"
		}

		marker := "  "
		if i == p.cursor {
			marker = "▸ "
		}

		line := fmt.Sprintf("%s%s %s %s %s",
			marker,
			hashStyle.Render(fmt.Sprintf("%-*s", hashWidth, hash)),
			authorStyle.Render(fmt.Sprintf("%-*s", authorWidth, author)),
			dateStyle.Render(fmt.Sprintf("%-*s", dateWidth, date)),
			msgStyle.Render(message),
		)

		if i == p.cursor {
			padLen := width - lipgloss.Width(line)
			if padLen > 0 {
				line += strings.Repeat(" ", padLen)
			}
			line = selectedStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (p *commitsPage) viewDiff(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		headerStyle.Render(p.diffHeader),
		dimStyle.Render("(esc back, j/k scroll)")))
	b.WriteString("\n")

	if p.diff == "" {
		b.WriteString("  Empty diff (no changes).")
		return b.String()
	}

	lines := strings.Split(p.diff, "\n")

	contentHeight := height - 3
	if contentHeight < 1 {
		contentHeight = 1
	}

	maxOffset := len(lines) - contentHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.diffOffset > maxOffset {
		p.diffOffset = maxOffset
	}

	end := p.diffOffset + contentHeight
	if end > len(lines) {
		end = len(lines)
	}

	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	for _, line := range lines[p.diffOffset:end] {
		displayLine := line
		if len(displayLine) > width-4 {
			displayLine = displayLine[:width-4]
		}

		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			b.WriteString("  ")
			b.WriteString(fileStyle.Render(displayLine))
		case strings.HasPrefix(line, "+"):
			b.WriteString("  ")
			b.WriteString(addStyle.Render(displayLine))
		case strings.HasPrefix(line, "-"):
			b.WriteString("  ")
			b.WriteString(delStyle.Render(displayLine))
		case strings.HasPrefix(line, "@@"):
			b.WriteString("  ")
			b.WriteString(hunkStyle.Render(displayLine))
		case strings.HasPrefix(line, "diff "):
			b.WriteString("  ")
			b.WriteString(fileStyle.Render(displayLine))
		default:
			b.WriteString("  ")
			b.WriteString(dimStyle.Render(displayLine))
		}
		b.WriteString("\n")
	}

	if len(lines) > contentHeight {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  line %d/%d", p.diffOffset+1, len(lines))))
	}

	return b.String()
}
