package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Page is the interface that all tab pages implement.
type Page interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Page, tea.Cmd)
	View(width, height int) string
}

// Tab identifiers.
const (
	TabSummary      = 0
	TabActivity     = 1
	TabContributors = 2
	TabBranches     = 3
	TabHealth       = 4
	TabCommits      = 5
)

var tabNames = []string{"Summary", "Activity", "Contributors", "Branches", "Health", "Commits"}

// Shared styles.
var (
	accentColor = lipgloss.Color("205")
	dimColor    = lipgloss.Color("241")
	brightColor = lipgloss.Color("255")
	mutedColor  = lipgloss.Color("245")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	dimStyle   = lipgloss.NewStyle().Faint(true)
	boldStyle  = lipgloss.NewStyle().Bold(true).Foreground(brightColor)
	mutedStyle = lipgloss.NewStyle().Foreground(mutedColor)

	// Chart colors.
	chartBlocks   = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	chartGradient = []lipgloss.Color{
		lipgloss.Color("63"),
		lipgloss.Color("33"),
		lipgloss.Color("39"),
		lipgloss.Color("49"),
		lipgloss.Color("82"),
		lipgloss.Color("154"),
	}
	chartPadding = 10
)

// renderBarChart renders a full bar chart of commit stats.
// width and height are the available space for the chart.
func renderBarChart(allStats []DayStat, granularity Granularity, width, height int) string {
	if len(allStats) == 0 {
		return "\n  No commit data."
	}

	cols := width - chartPadding
	if cols < 10 {
		cols = 10
	}

	stats := allStats
	if len(stats) > cols {
		stats = stats[len(stats)-cols:]
	}

	chartHeight := height - 5 // subtitle + padding + x-axis + months + footer
	if chartHeight < 3 {
		chartHeight = 3
	}
	if chartHeight > 24 {
		chartHeight = 24
	}

	maxCount := 0
	peakDate := time.Time{}
	totalCommits := 0
	for _, s := range stats {
		totalCommits += s.Count
		if s.Count > maxCount {
			maxCount = s.Count
			peakDate = s.Date
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	units := make([]int, len(stats))
	totalUnits := chartHeight * 8
	for i, s := range stats {
		units[i] = s.Count * totalUnits / maxCount
	}

	rowStyles := make([]lipgloss.Style, chartHeight)
	for r := 0; r < chartHeight; r++ {
		ci := (chartHeight - 1 - r) * len(chartGradient) / chartHeight
		if ci >= len(chartGradient) {
			ci = len(chartGradient) - 1
		}
		rowStyles[r] = lipgloss.NewStyle().Foreground(chartGradient[ci])
	}

	axisWidth := len(fmt.Sprintf("%d", maxCount))
	if axisWidth < 3 {
		axisWidth = 3
	}

	var b strings.Builder

	// Subtitle with granularity indicator.
	granLabels := []string{"[d]aily", "[w]eekly", "[m]onthly", "[y]early"}
	var granParts []string
	for i, l := range granLabels {
		if Granularity(i) == granularity {
			granParts = append(granParts, boldStyle.Render(l))
		} else {
			granParts = append(granParts, dimStyle.Render(l))
		}
	}
	b.WriteString(fmt.Sprintf("  %s  %s",
		dimStyle.Render("Commits"),
		strings.Join(granParts, dimStyle.Render(" / "))))
	b.WriteString("\n\n")

	for r := 0; r < chartHeight; r++ {
		label := ""
		if r == 0 {
			label = fmt.Sprintf("%d", maxCount)
		} else if r == chartHeight-1 {
			label = "0"
		} else if r == chartHeight/2 {
			label = fmt.Sprintf("%d", maxCount/2)
		}

		b.WriteString(fmt.Sprintf("  %*s ", axisWidth, label))
		if label != "" {
			b.WriteString("┤")
		} else {
			b.WriteString("│")
		}

		for c := 0; c < len(stats); c++ {
			u := units[c]
			rowLow := (chartHeight - 1 - r) * 8
			rowHigh := rowLow + 8

			var ch string
			if u >= rowHigh {
				ch = "█"
			} else if u > rowLow {
				ch = chartBlocks[u-rowLow]
			} else {
				ch = " "
			}
			b.WriteString(rowStyles[r].Render(ch))
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("  %*s └", axisWidth, ""))
	b.WriteString(strings.Repeat("─", len(stats)))
	b.WriteString("\n")

	monthLine := make([]byte, len(stats))
	for i := range monthLine {
		monthLine[i] = ' '
	}
	lastLabel := -10 // track last label position to avoid overlap
	for i, s := range stats {
		var label string
		showLabel := false

		switch granularity {
		case GranularityYearly:
			showLabel = i == 0 || stats[i].Date.Year() != stats[i-1].Date.Year()
			label = s.Date.Format("2006")
		case GranularityMonthly:
			showLabel = i == 0 || s.Date.Month() != stats[i-1].Date.Month()
			label = s.Date.Format("Jan")
			if s.Date.Month() == time.January || i == 0 {
				label = s.Date.Format("Jan 06")
			}
		default:
			showLabel = s.Date.Day() == 1 || i == 0
			label = s.Date.Format("Jan")
			if s.Date.Month() == time.January || i == 0 {
				label = s.Date.Format("Jan 06")
			}
		}

		if showLabel && i-lastLabel >= len(label)+1 {
			for j := 0; j < len(label) && i+j < len(monthLine); j++ {
				monthLine[i+j] = label[j]
			}
			lastLabel = i
		}
	}
	b.WriteString(fmt.Sprintf("  %*s  ", axisWidth, ""))
	b.WriteString(dimStyle.Render(string(monthLine)))
	b.WriteString("\n\n")

	peakStr := ""
	if !peakDate.IsZero() {
		peakFmt := peakDate.Format("Jan 2")
		if granularity == GranularityYearly {
			peakFmt = peakDate.Format("2006")
		} else if granularity == GranularityMonthly {
			peakFmt = peakDate.Format("Jan 2006")
		}
		peakStr = fmt.Sprintf("  Peak: %d (%s)", maxCount, peakFmt)
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Total: %d commits  %d %s periods%s",
		totalCommits, len(stats), granularity, peakStr)))

	return b.String()
}
