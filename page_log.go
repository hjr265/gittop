package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type commitsPage struct {
	commits []CommitInfo
	offset  int // scroll offset
}

func newCommitsPage() *commitsPage { return &commitsPage{} }

func (p *commitsPage) Init() tea.Cmd { return nil }

func (p *commitsPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case commitsDataMsg:
		p.commits = msg.commits
		if p.offset > len(p.commits)-1 {
			p.offset = 0
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if p.offset < len(p.commits)-1 {
				p.offset++
			}
		case "k", "up":
			if p.offset > 0 {
				p.offset--
			}
		case "g":
			p.offset = 0
		case "G":
			if len(p.commits) > 0 {
				p.offset = len(p.commits) - 1
			}
		}
	}
	return p, nil
}

func (p *commitsPage) View(width, height int) string {
	if len(p.commits) == 0 {
		return "\n  No commits in selected range."
	}

	var b strings.Builder
	b.WriteString("\n")

	hashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	authorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	dateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))

	// Header with count.
	b.WriteString(fmt.Sprintf("  %s\n\n",
		dimStyle.Render(fmt.Sprintf("%d commits  (j/k scroll, g/G top/bottom)", len(p.commits)))))

	visibleRows := height - 3 // header + blank lines
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Clamp offset so we always show a full page if possible.
	maxOffset := len(p.commits) - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}

	end := p.offset + visibleRows
	if end > len(p.commits) {
		end = len(p.commits)
	}

	// Determine available width for the message.
	// Format: "  <hash> <author> <date> <message>"
	// hash=7, author=max 16, date=10
	authorWidth := 16
	dateWidth := 10
	hashWidth := 7
	chrome := 2 + hashWidth + 1 + authorWidth + 1 + dateWidth + 1 // spaces between fields
	msgWidth := width - chrome
	if msgWidth < 10 {
		msgWidth = 10
	}

	for i := p.offset; i < end; i++ {
		c := p.commits[i]

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

		message := strings.TrimSpace(c.Message)
		// Take only first line.
		if idx := strings.IndexByte(message, '\n'); idx >= 0 {
			message = message[:idx]
		}
		if len(message) > msgWidth {
			message = message[:msgWidth-1] + "…"
		}

		b.WriteString(fmt.Sprintf("  %s %s %s %s\n",
			hashStyle.Render(fmt.Sprintf("%-*s", hashWidth, hash)),
			authorStyle.Render(fmt.Sprintf("%-*s", authorWidth, author)),
			dateStyle.Render(fmt.Sprintf("%-*s", dateWidth, date)),
			msgStyle.Render(message),
		))
	}

	return b.String()
}
