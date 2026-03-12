package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type summaryPage struct {
	stats []DayStat
}

func newSummaryPage() *summaryPage {
	return &summaryPage{}
}

func (p *summaryPage) Init() tea.Cmd { return nil }

func (p *summaryPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if msg, ok := msg.(statsMsg); ok {
		p.stats = msg.stats
	}
	return p, nil
}

func (p *summaryPage) View(width, height int) string {
	if len(p.stats) == 0 {
		return "\n  No data."
	}

	var b strings.Builder

	// Compute KPIs.
	totalCommits := 0
	peakCount := 0
	peakDate := time.Time{}
	activeDays := 0
	for _, s := range p.stats {
		totalCommits += s.Count
		if s.Count > 0 {
			activeDays++
		}
		if s.Count > peakCount {
			peakCount = s.Count
			peakDate = s.Date
		}
	}

	// Time range.
	first := p.stats[0].Date
	last := p.stats[len(p.stats)-1].Date
	repoSpan := last.Sub(first)
	// KPI cards.
	cards := []struct {
		label string
		value string
	}{
		{"Total Commits", fmt.Sprintf("%d", totalCommits)},
		{"Active Days", fmt.Sprintf("%d / %d", activeDays, len(p.stats))},
		{"Peak Day", fmt.Sprintf("%d (%s)", peakCount, peakDate.Format("Jan 2"))},
		{"Time Span", fmt.Sprintf("%d days", int(repoSpan.Hours()/24))},
	}

	// Each card has 2 cols border + 2 cols padding = 4 cols of chrome.
	// Plus 2 cols left indent for the row.
	cardChrome := 4
	totalCardWidth := (width - 2) / len(cards)
	if totalCardWidth < 16 {
		totalCardWidth = 16
	}
	if totalCardWidth > 30 {
		totalCardWidth = 30
	}
	innerWidth := totalCardWidth - cardChrome
	if innerWidth < 8 {
		innerWidth = 8
	}

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Width(innerWidth).
		Padding(0, 1)

	cardValueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	cardLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	var cardViews []string
	for _, c := range cards {
		content := cardValueStyle.Render(c.value) + "\n" + cardLabelStyle.Render(c.label)
		cardViews = append(cardViews, cardStyle.Render(content))
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cardViews...))
	b.WriteString("\n\n")

	// Bar chart.
	b.WriteString(renderBarChart(p.stats, width, height-7))

	return b.String()
}
