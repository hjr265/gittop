package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type contribView int

const (
	viewLeaderboard contribView = iota
	viewCadence
	viewTimeline
	viewOwnership
	contribViewCount
)

var contribViewNames = []string{"Leaderboard", "Cadence", "Timeline", "Ownership"}

type authorInfo struct {
	Name        string
	Commits     int
	FirstCommit time.Time
	LastCommit  time.Time
	ActiveDays  int
	// Cadence: number of distinct weeks with at least one commit.
	ActiveWeeks int
	TotalWeeks  int // weeks in span from first to last commit
	// Ownership: number of files where this author has the most commits.
	OwnedFiles int
}

type contributorsPage struct {
	commits  []CommitInfo
	authors  []authorInfo
	view     contribView
	offset   int
	needFiles bool // true when ownership view needs file data
}

func newContributorsPage() *contributorsPage { return &contributorsPage{} }

func (p *contributorsPage) Init() tea.Cmd { return nil }

func (p *contributorsPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case commitsDataMsg:
		p.commits = msg.commits
		p.recompute()
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			p.view = (p.view + 1) % contribViewCount
			p.offset = 0
		case "j", "down":
			p.offset++
		case "k", "up":
			if p.offset > 0 {
				p.offset--
			}
		case "g":
			p.offset = 0
		case "G":
			if len(p.authors) > 0 {
				p.offset = len(p.authors) - 1
			}
		}
	}
	return p, nil
}

func (p *contributorsPage) recompute() {
	if len(p.commits) == 0 {
		p.authors = nil
		return
	}

	type authorData struct {
		name        string
		commits     int
		first, last time.Time
		daySet      map[time.Time]bool
		weekSet     map[string]bool // "2006-W02" keys
		fileCounts  map[string]int  // file -> commit count for this author
	}

	byAuthor := map[string]*authorData{}
	hasFiles := false

	for i := range p.commits {
		c := &p.commits[i]
		key := c.Author
		ad, ok := byAuthor[key]
		if !ok {
			ad = &authorData{
				name:       c.Author,
				first:      c.Date,
				last:       c.Date,
				daySet:     map[time.Time]bool{},
				weekSet:    map[string]bool{},
				fileCounts: map[string]int{},
			}
			byAuthor[key] = ad
		}
		ad.commits++
		if c.Date.Before(ad.first) {
			ad.first = c.Date
		}
		if c.Date.After(ad.last) {
			ad.last = c.Date
		}
		ad.daySet[c.Date] = true
		y, w := c.Date.ISOWeek()
		ad.weekSet[fmt.Sprintf("%d-W%02d", y, w)] = true

		if len(c.Files) > 0 {
			hasFiles = true
			for _, f := range c.Files {
				ad.fileCounts[f]++
			}
		}
	}

	// Compute file ownership: for each file, find who has the most commits.
	ownerCounts := map[string]int{}
	if hasFiles {
		fileAuthors := map[string]map[string]int{}
		for authorName, ad := range byAuthor {
			for file, count := range ad.fileCounts {
				if fileAuthors[file] == nil {
					fileAuthors[file] = map[string]int{}
				}
				fileAuthors[file][authorName] += count
			}
		}
		for _, authors := range fileAuthors {
			bestAuthor := ""
			bestCount := 0
			for a, c := range authors {
				if c > bestCount {
					bestCount = c
					bestAuthor = a
				}
			}
			if bestAuthor != "" {
				ownerCounts[bestAuthor]++
			}
		}
	}

	p.needFiles = !hasFiles
	var authors []authorInfo
	for _, ad := range byAuthor {
		totalWeeks := int(ad.last.Sub(ad.first).Hours()/24/7) + 1
		if totalWeeks < 1 {
			totalWeeks = 1
		}
		authors = append(authors, authorInfo{
			Name:        ad.name,
			Commits:     ad.commits,
			FirstCommit: ad.first,
			LastCommit:  ad.last,
			ActiveDays:  len(ad.daySet),
			ActiveWeeks: len(ad.weekSet),
			TotalWeeks:  totalWeeks,
			OwnedFiles:  ownerCounts[ad.name],
		})
	}
	sort.Slice(authors, func(i, j int) bool { return authors[i].Commits > authors[j].Commits })
	p.authors = authors
}

func (p *contributorsPage) View(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	// View selector.
	var viewParts []string
	for i, name := range contribViewNames {
		if contribView(i) == p.view {
			viewParts = append(viewParts, boldStyle.Render(name))
		} else {
			viewParts = append(viewParts, dimStyle.Render(name))
		}
	}
	b.WriteString(fmt.Sprintf("  %s  %s", dimStyle.Render("[v]iew"), strings.Join(viewParts, dimStyle.Render(" / "))))
	b.WriteString("\n\n")

	if len(p.authors) == 0 {
		b.WriteString("  No data.")
		return b.String()
	}

	contentHeight := height - 3

	switch p.view {
	case viewLeaderboard:
		b.WriteString(p.renderLeaderboard(width, contentHeight))
	case viewCadence:
		b.WriteString(p.renderCadence(width, contentHeight))
	case viewTimeline:
		b.WriteString(p.renderTimeline(width, contentHeight))
	case viewOwnership:
		b.WriteString(p.renderOwnership(width, contentHeight))
	}

	return b.String()
}

func (p *contributorsPage) visibleAuthors(height int) []authorInfo {
	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	authors := p.authors
	if p.offset >= len(authors) {
		p.offset = len(authors) - 1
	}
	if p.offset < 0 {
		p.offset = 0
	}

	// Clamp offset.
	maxOffset := len(authors) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}

	end := p.offset + maxRows
	if end > len(authors) {
		end = len(authors)
	}
	return authors[p.offset:end]
}

func (p *contributorsPage) renderLeaderboard(width, height int) string {
	visible := p.visibleAuthors(height)
	if len(visible) == 0 {
		return "  No data."
	}

	maxCommits := p.authors[0].Commits
	if maxCommits == 0 {
		maxCommits = 1
	}

	nameWidth := 0
	for _, a := range visible {
		if l := len(a.Name); l > nameWidth {
			nameWidth = l
		}
	}
	if nameWidth > width/3 {
		nameWidth = width / 3
	}

	countLabel := func(a *authorInfo) string {
		return fmt.Sprintf("%d commits", a.Commits)
	}
	labelWidth := 0
	for i := range visible {
		if l := len(countLabel(&visible[i])); l > labelWidth {
			labelWidth = l
		}
	}

	barMaxWidth := width - nameWidth - labelWidth - 8
	if barMaxWidth < 5 {
		barMaxWidth = 5
	}

	barGradient := []lipgloss.Color{
		lipgloss.Color("22"),
		lipgloss.Color("28"),
		lipgloss.Color("34"),
		lipgloss.Color("82"),
		lipgloss.Color("154"),
	}

	var b strings.Builder
	for i := range visible {
		a := &visible[i]
		name := a.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		ci := a.Commits * (len(barGradient) - 1) / maxCommits
		if ci >= len(barGradient) {
			ci = len(barGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(barGradient[ci])

		b.WriteString(fmt.Sprintf("  %-*s ", nameWidth, name))
		bar, barW := smoothBar(a.Commits, maxCommits, barMaxWidth, barStyle)
		b.WriteString(bar)
		pad := barMaxWidth - barW
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %*s", labelWidth, countLabel(a))))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d contributors, %d total commits", len(p.authors), func() int {
		t := 0
		for _, a := range p.authors {
			t += a.Commits
		}
		return t
	}())))

	return b.String()
}

func (p *contributorsPage) renderCadence(width, height int) string {
	// Sort by consistency (active weeks / total weeks).
	sorted := make([]authorInfo, len(p.authors))
	copy(sorted, p.authors)
	sort.Slice(sorted, func(i, j int) bool {
		ci := float64(sorted[i].ActiveWeeks) / float64(sorted[i].TotalWeeks)
		cj := float64(sorted[j].ActiveWeeks) / float64(sorted[j].TotalWeeks)
		if ci == cj {
			return sorted[i].Commits > sorted[j].Commits
		}
		return ci > cj
	})

	// Use sorted list for visible computation.
	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if p.offset >= len(sorted) {
		p.offset = len(sorted) - 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := len(sorted) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
	end := p.offset + maxRows
	if end > len(sorted) {
		end = len(sorted)
	}
	visible := sorted[p.offset:end]

	if len(visible) == 0 {
		return "  No data."
	}

	nameWidth := 0
	for _, a := range visible {
		if l := len(a.Name); l > nameWidth {
			nameWidth = l
		}
	}
	if nameWidth > width/3 {
		nameWidth = width / 3
	}

	barMaxWidth := width - nameWidth - 30
	if barMaxWidth < 5 {
		barMaxWidth = 5
	}

	var b strings.Builder
	for _, a := range visible {
		name := a.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		consistency := float64(a.ActiveWeeks) / float64(a.TotalWeeks) * 100
		if a.TotalWeeks <= 1 {
			consistency = 100
		}

		var barStyle lipgloss.Style
		switch {
		case consistency >= 75:
			barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
		case consistency >= 50:
			barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		case consistency >= 25:
			barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
		default:
			barStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		}

		b.WriteString(fmt.Sprintf("  %-*s ", nameWidth, name))
		bar, barW := smoothBar(int(consistency), 100, barMaxWidth, barStyle)
		b.WriteString(bar)
		pad := barMaxWidth - barW
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}

		label := fmt.Sprintf(" %3.0f%% (%d/%dw)", consistency, a.ActiveWeeks, a.TotalWeeks)
		b.WriteString(dimStyle.Render(label))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Consistency = active weeks / total span in weeks"))

	return b.String()
}

func (p *contributorsPage) renderTimeline(width, height int) string {
	if len(p.authors) == 0 {
		return "  No data."
	}

	// Sort by first commit (earliest first).
	sorted := make([]authorInfo, len(p.authors))
	copy(sorted, p.authors)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].FirstCommit.Before(sorted[j].FirstCommit)
	})

	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if p.offset >= len(sorted) {
		p.offset = len(sorted) - 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := len(sorted) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
	end := p.offset + maxRows
	if end > len(sorted) {
		end = len(sorted)
	}
	visible := sorted[p.offset:end]

	if len(visible) == 0 {
		return "  No data."
	}

	// Find global time range.
	globalFirst := sorted[0].FirstCommit
	globalLast := sorted[0].LastCommit
	for _, a := range sorted {
		if a.FirstCommit.Before(globalFirst) {
			globalFirst = a.FirstCommit
		}
		if a.LastCommit.After(globalLast) {
			globalLast = a.LastCommit
		}
	}
	now := time.Now()
	if now.After(globalLast) {
		globalLast = now
	}
	globalSpan := globalLast.Sub(globalFirst).Hours() / 24
	if globalSpan < 1 {
		globalSpan = 1
	}

	nameWidth := 0
	for _, a := range visible {
		if l := len(a.Name); l > nameWidth {
			nameWidth = l
		}
	}
	if nameWidth > width/4 {
		nameWidth = width / 4
	}

	dateWidth := 22 // "Jan 02 06 - Jan 02 06"
	barWidth := width - nameWidth - dateWidth - 6
	if barWidth < 10 {
		barWidth = 10
	}

	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dormantStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	var b strings.Builder
	for _, a := range visible {
		name := a.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		// Compute bar position.
		startPos := int(a.FirstCommit.Sub(globalFirst).Hours() / 24 * float64(barWidth) / globalSpan)
		endPos := int(a.LastCommit.Sub(globalFirst).Hours() / 24 * float64(barWidth) / globalSpan)
		if startPos < 0 {
			startPos = 0
		}
		if endPos >= barWidth {
			endPos = barWidth - 1
		}
		if endPos < startPos {
			endPos = startPos
		}
		spanLen := endPos - startPos + 1
		if spanLen < 1 {
			spanLen = 1
		}

		// Is this author dormant? (no commits in last 90 days)
		daysSinceLast := int(now.Sub(a.LastCommit).Hours() / 24)
		style := activeStyle
		if daysSinceLast > 90 {
			style = dormantStyle
		}

		b.WriteString(fmt.Sprintf("  %-*s ", nameWidth, name))

		if startPos > 0 {
			b.WriteString(strings.Repeat(" ", startPos))
		}
		b.WriteString(style.Render(strings.Repeat("━", spanLen)))
		pad := barWidth - startPos - spanLen
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}

		dates := fmt.Sprintf(" %s - %s", a.FirstCommit.Format("Jan 02 06"), a.LastCommit.Format("Jan 02 06"))
		b.WriteString(dimStyle.Render(dates))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	legend := fmt.Sprintf("  %s active  %s dormant (>90d)",
		activeStyle.Render("━"),
		dormantStyle.Render("━"))
	b.WriteString(legend)

	return b.String()
}

func (p *contributorsPage) renderOwnership(width, height int) string {
	if p.needFiles {
		return "  Loading file data for ownership analysis..."
	}

	// Sort by owned files.
	sorted := make([]authorInfo, len(p.authors))
	copy(sorted, p.authors)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].OwnedFiles == sorted[j].OwnedFiles {
			return sorted[i].Commits > sorted[j].Commits
		}
		return sorted[i].OwnedFiles > sorted[j].OwnedFiles
	})

	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if p.offset >= len(sorted) {
		p.offset = len(sorted) - 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := len(sorted) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
	end := p.offset + maxRows
	if end > len(sorted) {
		end = len(sorted)
	}
	visible := sorted[p.offset:end]

	if len(visible) == 0 {
		return "  No data."
	}

	// Total files for percentage.
	totalFiles := 0
	for _, a := range sorted {
		totalFiles += a.OwnedFiles
	}
	if totalFiles == 0 {
		return "  No file ownership data. Waiting for file data..."
	}

	maxOwned := sorted[0].OwnedFiles
	if maxOwned == 0 {
		maxOwned = 1
	}

	nameWidth := 0
	for _, a := range visible {
		if l := len(a.Name); l > nameWidth {
			nameWidth = l
		}
	}
	if nameWidth > width/3 {
		nameWidth = width / 3
	}

	labelWidth := 16 // "123 files (45%)"
	barMaxWidth := width - nameWidth - labelWidth - 8
	if barMaxWidth < 5 {
		barMaxWidth = 5
	}

	barGradient := []lipgloss.Color{
		lipgloss.Color("63"),
		lipgloss.Color("33"),
		lipgloss.Color("39"),
		lipgloss.Color("82"),
		lipgloss.Color("154"),
	}

	var b strings.Builder
	for _, a := range visible {
		name := a.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		ci := a.OwnedFiles * (len(barGradient) - 1) / maxOwned
		if ci >= len(barGradient) {
			ci = len(barGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(barGradient[ci])

		pct := float64(a.OwnedFiles) * 100 / float64(totalFiles)

		b.WriteString(fmt.Sprintf("  %-*s ", nameWidth, name))
		bar, barW := smoothBar(a.OwnedFiles, maxOwned, barMaxWidth, barStyle)
		b.WriteString(bar)
		pad := barMaxWidth - barW
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		label := fmt.Sprintf("%d files (%s%%)", a.OwnedFiles, formatPct(pct))
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %*s", labelWidth, label)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Owner = author with most commits to file (%d files total)", totalFiles)))

	return b.String()
}

func formatPct(pct float64) string {
	if pct >= 10 {
		return fmt.Sprintf("%.0f", pct)
	}
	if pct >= 1 {
		return fmt.Sprintf("%.1f", pct)
	}
	return fmt.Sprintf("%.1f", math.Max(pct, 0.1))
}

func placeholderView(title string, width, height int) string {
	titleRendered := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		Render(title)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Foreground(lipgloss.Color("245")).
		Width(30).
		Padding(1, 2).
		Render(fmt.Sprintf("%s\n\n%s",
			titleRendered,
			dimStyle.Render("Coming soon"),
		))

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
