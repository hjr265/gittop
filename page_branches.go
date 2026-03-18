package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type branchSortCol int

const (
	sortByName branchSortCol = iota
	sortByLastCommit
	sortByAuthor
	sortByAhead
	sortByBehind
	branchSortCount
)

var branchColNames = []string{"Name", "Last Commit", "Author", "Ahead", "Behind"}

type branchesDataMsg struct {
	branches []BranchInfo
	err      error
}

type branchesPage struct {
	branches []BranchInfo
	sortCol  branchSortCol
	sortAsc  bool
	offset   int
	loaded   bool
}

func newBranchesPage() *branchesPage {
	return &branchesPage{sortCol: sortByLastCommit, sortAsc: false}
}

func (p *branchesPage) Init() tea.Cmd { return nil }

func (p *branchesPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case branchesDataMsg:
		if msg.err == nil {
			p.branches = msg.branches
			p.loaded = true
			p.sortBranches()
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "s":
			p.sortCol = (p.sortCol + 1) % branchSortCount
			p.sortBranches()
			p.offset = 0
		case "S":
			p.sortAsc = !p.sortAsc
			p.sortBranches()
		case "j", "down":
			if p.offset < len(p.branches)-1 {
				p.offset++
			}
		case "k", "up":
			if p.offset > 0 {
				p.offset--
			}
		case "g":
			p.offset = 0
		case "G":
			if len(p.branches) > 0 {
				p.offset = len(p.branches) - 1
			}
		}
	}
	return p, nil
}

func (p *branchesPage) sortBranches() {
	asc := p.sortAsc
	sort.SliceStable(p.branches, func(i, j int) bool {
		var less bool
		switch p.sortCol {
		case sortByName:
			less = p.branches[i].Name < p.branches[j].Name
		case sortByLastCommit:
			less = p.branches[i].LastCommit.After(p.branches[j].LastCommit)
		case sortByAuthor:
			less = p.branches[i].Author < p.branches[j].Author
		case sortByAhead:
			less = p.branches[i].Ahead > p.branches[j].Ahead
		case sortByBehind:
			less = p.branches[i].Behind > p.branches[j].Behind
		}
		if asc {
			return !less
		}
		return less
	})
}

func (p *branchesPage) View(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	// Sort indicator.
	var sortParts []string
	for i, name := range branchColNames {
		if branchSortCol(i) == p.sortCol {
			arrow := "▼"
			if p.sortAsc {
				arrow = "▲"
			}
			sortParts = append(sortParts, boldStyle.Render(name+" "+arrow))
		} else {
			sortParts = append(sortParts, dimStyle.Render(name))
		}
	}
	b.WriteString(fmt.Sprintf("  %s  %s", dimStyle.Render("[s]ort"), strings.Join(sortParts, dimStyle.Render(" / "))))
	b.WriteString("\n\n")

	if !p.loaded {
		b.WriteString("  Loading branches...")
		return b.String()
	}

	if len(p.branches) == 0 {
		b.WriteString("  No branches found.")
		return b.String()
	}

	contentHeight := height - 3

	// Column widths.
	nameWidth := 0
	authorWidth := 0
	for _, br := range p.branches {
		if l := len(br.Name); l > nameWidth {
			nameWidth = l
		}
		if l := len(br.Author); l > authorWidth {
			authorWidth = l
		}
	}
	// Add space for current branch marker.
	nameWidth += 2
	if nameWidth > width/3 {
		nameWidth = width / 3
	}
	if authorWidth > width/4 {
		authorWidth = width / 4
	}

	dateWidth := 12 // "Mar 14, 2026"
	aheadWidth := 7
	behindWidth := 7

	// Visible rows.
	maxRows := contentHeight - 2 // header + footer
	if maxRows < 1 {
		maxRows = 1
	}
	if p.offset >= len(p.branches) {
		p.offset = len(p.branches) - 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := len(p.branches) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
	end := p.offset + maxRows
	if end > len(p.branches) {
		end = len(p.branches)
	}

	// Header.
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	hBranch := headerStyle.Render("Branch")
	hDate := headerStyle.Render("Last Commit")
	hAuthor := headerStyle.Render("Author")
	hAhead := headerStyle.Render("Ahead")
	hBehind := headerStyle.Render("Behind")
	hStatus := headerStyle.Render("Status")
	b.WriteString(fmt.Sprintf("  %s%s %s%s %s%s %s%s %s%s  %s\n",
		hBranch, strings.Repeat(" ", max(0, nameWidth-2-len("Branch"))),
		hDate, strings.Repeat(" ", max(0, dateWidth-len("Last Commit"))),
		hAuthor, strings.Repeat(" ", max(0, authorWidth-len("Author"))),
		strings.Repeat(" ", max(0, aheadWidth-len("Ahead"))), hAhead,
		strings.Repeat(" ", max(0, behindWidth-len("Behind"))), hBehind,
		hStatus,
	))

	now := time.Now()
	currentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	aheadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	behindStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	zeroStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	for i := p.offset; i < end; i++ {
		br := p.branches[i]

		name := br.Name
		maxName := nameWidth - 2 // room for marker
		if len(name) > maxName {
			name = name[:maxName-1] + "…"
		}
		marker := "  "
		if br.IsCurrent {
			marker = "* "
			name = currentStyle.Render(name)
		} else {
			name = nameStyle.Render(name)
		}
		displayName := marker + name
		// Pad name column accounting for ANSI.
		nameVisual := 2 + min(len(br.Name), maxName) // marker + visible name
		namePad := nameWidth - nameVisual
		if namePad < 0 {
			namePad = 0
		}

		date := formatRelativeDate(now, br.LastCommit)
		datePad := dateWidth - len(date)
		if datePad < 0 {
			datePad = 0
		}

		author := br.Author
		if len(author) > authorWidth {
			author = author[:authorWidth-1] + "…"
		}
		authorPad := authorWidth - len(author)
		if authorPad < 0 {
			authorPad = 0
		}

		aheadStr := zeroStyle.Render(fmt.Sprintf("%*d", aheadWidth, br.Ahead))
		if br.Ahead > 0 {
			aheadStr = aheadStyle.Render(fmt.Sprintf("%*d", aheadWidth, br.Ahead))
		}
		behindStr := zeroStyle.Render(fmt.Sprintf("%*d", behindWidth, br.Behind))
		if br.Behind > 0 {
			behindStr = behindStyle.Render(fmt.Sprintf("%*d", behindWidth, br.Behind))
		}

		// Status tags.
		var statusTags []string
		staleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
		goneStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		if br.Stale && !br.IsCurrent {
			statusTags = append(statusTags, staleStyle.Render("stale"))
		}
		if br.RemoteGone {
			statusTags = append(statusTags, goneStyle.Render("gone"))
		}
		status := strings.Join(statusTags, " ")

		b.WriteString(fmt.Sprintf("%s%s %s%s %s%s %s %s  %s\n",
			displayName, strings.Repeat(" ", namePad),
			dimStyle.Render(date), strings.Repeat(" ", datePad),
			mutedStyle.Render(author), strings.Repeat(" ", authorPad),
			aheadStr,
			behindStr,
			status,
		))
	}

	b.WriteString("\n")
	staleCount := 0
	goneCount := 0
	for _, br := range p.branches {
		if br.Stale && !br.IsCurrent {
			staleCount++
		}
		if br.RemoteGone {
			goneCount++
		}
	}
	footer := fmt.Sprintf("  %d branches", len(p.branches))
	if staleCount > 0 {
		footer += fmt.Sprintf(" · %d stale", staleCount)
	}
	if goneCount > 0 {
		footer += fmt.Sprintf(" · %d gone", goneCount)
	}
	b.WriteString(dimStyle.Render(footer))

	return b.String()
}

func formatRelativeDate(now time.Time, t time.Time) string {
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 7:
		return fmt.Sprintf("%dd ago", days)
	case days < 30:
		return fmt.Sprintf("%dw ago", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo ago", days/30)
	default:
		return t.Format("Jan 2006")
	}
}
