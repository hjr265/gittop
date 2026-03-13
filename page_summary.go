package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Range presets for the chart.
type timeRange struct {
	label string
	days  int // 0 means "all time"
}

var rangePresets = []timeRange{
	{"3m", 90},
	{"6m", 180},
	{"1y", 365},
	{"2y", 730},
	{"5y", 1825},
	{"all", 0},
}

const defaultRangeIdx = 2 // 1y

type summaryPage struct {
	stats       []DayStat
	granularity Granularity
}

func newSummaryPage() *summaryPage {
	return &summaryPage{}
}

func (p *summaryPage) Init() tea.Cmd { return nil }

func (p *summaryPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case statsMsg:
		p.stats = msg.stats
	case tea.KeyMsg:
		switch msg.String() {
		case "d":
			p.granularity = GranularityDaily
		case "w":
			p.granularity = GranularityWeekly
		case "m":
			p.granularity = GranularityMonthly
		case "y":
			p.granularity = GranularityYearly
		case "g":
			p.granularity = p.granularity.Next()
		}
	}
	return p, nil
}

func (p *summaryPage) View(width, height int) string {
	if len(p.stats) == 0 {
		return "\n  No data in selected range."
	}

	stats := p.stats

	var b strings.Builder

	// Compute KPIs from filtered stats.
	totalCommits := 0
	peakCount := 0
	peakDate := time.Time{}
	activeDays := 0
	for _, s := range stats {
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
	first := stats[0].Date
	last := stats[len(stats)-1].Date
	repoSpan := last.Sub(first)
	// KPI cards.
	cards := []struct {
		label string
		value string
	}{
		{"Total Commits", fmt.Sprintf("%d", totalCommits)},
		{"Active Days", fmt.Sprintf("%d / %d", activeDays, len(stats))},
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
	aggregated := AggregateStats(stats, p.granularity)
	b.WriteString(renderBarChart(aggregated, p.granularity, width, height-7))

	return b.String()
}
