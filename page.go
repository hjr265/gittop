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
	TabReleases     = 5
	TabCommits      = 6
)

var tabNames = []string{"Summary", "Activity", "Contributors", "Branches", "Health", "Releases", "Commits"}

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
	chartGradient = []lipgloss.Color{
		lipgloss.Color("63"),
		lipgloss.Color("33"),
		lipgloss.Color("39"),
		lipgloss.Color("49"),
		lipgloss.Color("82"),
		lipgloss.Color("154"),
	}
	chartPadding = 10

	// Braille area chart: fill patterns for left and right dot columns.
	// Each braille cell is 2 dots wide × 4 dots tall.
	// Left column pins (bottom to top): 7(0x40), 3(0x04), 2(0x02), 1(0x01)
	// Right column pins (bottom to top): 8(0x80), 6(0x20), 5(0x10), 4(0x08)
	brailleLeftFill  = [5]int{0x00, 0x40, 0x44, 0x46, 0x47}
	brailleRightFill = [5]int{0x00, 0x80, 0xA0, 0xB0, 0xB8}

	// Horizontal fractional blocks for smooth bar endings.
	hFracBlocks = []string{" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
)

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// smoothBar renders a horizontal bar with sub-character precision using
// fractional block characters. Returns the rendered string and its visual
// width in character cells.
func smoothBar(value, maxValue, maxWidth int, style lipgloss.Style) (string, int) {
	if maxValue == 0 || value == 0 || maxWidth == 0 {
		return "", 0
	}
	w8 := value * maxWidth * 8 / maxValue
	if w8 > maxWidth*8 {
		w8 = maxWidth * 8
	}
	full := w8 / 8
	frac := w8 % 8
	charWidth := full

	var b strings.Builder
	if full > 0 {
		b.WriteString(style.Render(strings.Repeat("█", full)))
	}
	if frac > 0 {
		b.WriteString(style.Render(hFracBlocks[frac]))
		charWidth++
	}
	return b.String(), charWidth
}

// renderBarChart renders a braille area chart of commit stats.
// Uses Unicode braille characters for smooth, btop-style rendering
// with 2× horizontal and 4× vertical sub-character resolution.
func renderBarChart(allStats []DayStat, granularity Granularity, width, height int) string {
	if len(allStats) == 0 {
		return "\n  No commit data."
	}

	cols := width - chartPadding
	if cols < 10 {
		cols = 10
	}

	// Each braille cell covers 2 data points, so we can show 2× as many.
	maxDataPoints := cols * 2
	stats := allStats
	if len(stats) > maxDataPoints {
		stats = stats[len(stats)-maxDataPoints:]
	}

	chartHeight := height - 5 // subtitle + padding + x-axis + months + footer
	if chartHeight < 3 {
		chartHeight = 3
	}
	if chartHeight > 24 {
		chartHeight = 24
	}

	totalDotRows := chartHeight * 4

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

	// Scale each data point to dot-row units.
	dotHeights := make([]int, len(stats))
	for i, s := range stats {
		dotHeights[i] = s.Count * totalDotRows / maxCount
	}

	charCols := (len(stats) + 1) / 2

	// Row gradient styles (row 0 = top of chart).
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

	// Render chart rows using braille characters.
	for r := 0; r < chartHeight; r++ {
		// Y-axis label.
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

		rowBottom := (chartHeight - 1 - r) * 4

		for cc := 0; cc < charCols; cc++ {
			leftIdx := cc * 2
			rightIdx := cc*2 + 1

			leftH := 0
			if leftIdx < len(dotHeights) {
				leftH = dotHeights[leftIdx]
			}
			rightH := 0
			if rightIdx < len(dotHeights) {
				rightH = dotHeights[rightIdx]
			}

			ld := clampInt(leftH-rowBottom, 0, 4)
			rd := clampInt(rightH-rowBottom, 0, 4)

			if ld == 0 && rd == 0 {
				b.WriteRune(' ')
			} else {
				ch := rune(0x2800 + brailleLeftFill[ld] + brailleRightFill[rd])
				b.WriteString(rowStyles[r].Render(string(ch)))
			}
		}
		b.WriteString("\n")
	}

	// X-axis.
	b.WriteString(fmt.Sprintf("  %*s └", axisWidth, ""))
	b.WriteString(strings.Repeat("─", charCols))
	b.WriteString("\n")

	// Month labels (adjusted for braille: 2 data points per character cell).
	monthLine := make([]byte, charCols)
	for i := range monthLine {
		monthLine[i] = ' '
	}
	lastLabel := -10
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

		charPos := i / 2
		if showLabel && charPos-lastLabel >= len(label)+1 {
			for j := 0; j < len(label) && charPos+j < len(monthLine); j++ {
				monthLine[charPos+j] = label[j]
			}
			lastLabel = charPos
		}
	}
	b.WriteString(fmt.Sprintf("  %*s  ", axisWidth, ""))
	b.WriteString(dimStyle.Render(string(monthLine)))
	b.WriteString("\n\n")

	// Footer.
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
