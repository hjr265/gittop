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
	TabFiles        = 4
	TabLog          = 5
)

var tabNames = []string{"Summary", "Activity", "Contributors", "Branches", "Files", "Log"}

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

// renderBarChart renders a full bar chart of daily commit stats.
// width and height are the available space for the chart.
func renderBarChart(allStats []DayStat, width, height int) string {
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

	b.WriteString(dimStyle.Render(fmt.Sprintf("  Commits over the last %d days", len(stats))))
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
	for i, s := range stats {
		if s.Date.Day() == 1 || i == 0 {
			label := s.Date.Format("Jan")
			if s.Date.Month() == time.January || i == 0 {
				label = s.Date.Format("Jan 06")
			}
			for j := 0; j < len(label) && i+j < len(monthLine); j++ {
				monthLine[i+j] = label[j]
			}
		}
	}
	b.WriteString(fmt.Sprintf("  %*s  ", axisWidth, ""))
	b.WriteString(dimStyle.Render(string(monthLine)))
	b.WriteString("\n\n")

	peakStr := ""
	if !peakDate.IsZero() {
		peakStr = fmt.Sprintf("  Peak: %d (%s)", maxCount, peakDate.Format("Jan 2"))
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Total: %d commits%s", totalCommits, peakStr)))

	return b.String()
}
