package main

import (
	"fmt"
	"math"
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

// GraphSymbol selects the rendering style for area charts.
type GraphSymbol int

const (
	GraphBraille GraphSymbol = iota
	GraphBlock
)

// graphSymbolMsg is sent when the user changes the graph symbol setting.
type graphSymbolMsg struct {
	symbol GraphSymbol
}

// Tab identifiers.
const (
	TabSummary      = 0
	TabActivity     = 1
	TabContributors = 2
	TabBranches     = 3
	TabFiles        = 4
	TabReleases     = 5
	TabCommits      = 6
)

var tabNames = []string{"Summary", "Activity", "Contributors", "Branches", "Files", "Releases", "Commits"}

// Shared styles — set by ApplyTheme().
var (
	accentColor lipgloss.Color
	dimColor    lipgloss.Color
	brightColor lipgloss.Color
	mutedColor  lipgloss.Color

	dimStyle lipgloss.Style
	boldStyle  lipgloss.Style
	mutedStyle lipgloss.Style

	chartGradient []lipgloss.Color
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

// Vertical block characters for block-based charts (1/8 to full block).
var vBlockChars = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// renderBarChart renders an area chart of commit stats.
func renderBarChart(stats []DayStat, granularity Granularity, width, height int, symbol GraphSymbol) string {
	if len(stats) == 0 {
		return "\n  No commit data."
	}

	cols := width - chartPadding
	if cols < 10 {
		cols = 10
	}

	// Braille packs 2 data points per cell; block uses 1.
	pointsPerCell := 1
	subRows := 8 // block: 8 sub-rows per character (▁▂▃▄▅▆▇█)
	if symbol == GraphBraille {
		pointsPerCell = 2
		subRows = 4
	}
	slots := cols * pointsPerCell
	values := resampleCounts(stats, slots)

	chartHeight := height - 5 // subtitle + padding + x-axis + months + footer
	if chartHeight < 3 {
		chartHeight = 3
	}
	if chartHeight > 24 {
		chartHeight = 24
	}

	totalDotRows := chartHeight * subRows

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

	maxValue := 0.0
	for _, v := range values {
		maxValue = max(maxValue, v)
	}
	if maxValue == 0 {
		maxValue = 1
	}

	// Scale each slot to dot-row units.
	dotHeights := make([]int, len(values))
	for i, v := range values {
		dotHeights[i] = int(v * float64(totalDotRows) / maxValue)
	}

	charCols := cols

	// Row gradient styles (row 0 = top of chart).
	rowStyles := make([]lipgloss.Style, chartHeight)
	for r := 0; r < chartHeight; r++ {
		ci := (chartHeight - 1 - r) * len(chartGradient) / chartHeight
		if ci >= len(chartGradient) {
			ci = len(chartGradient) - 1
		}
		rowStyles[r] = lipgloss.NewStyle().Foreground(chartGradient[ci])
	}

	topLabel := formatAxisValue(maxValue)
	midLabel := formatAxisValue(maxValue / 2)
	axisWidth := max(len(topLabel), len(midLabel), 3)

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

	// Render chart rows.
	for r := 0; r < chartHeight; r++ {
		// Y-axis label.
		label := ""
		if r == 0 {
			label = topLabel
		} else if r == chartHeight-1 {
			label = "0"
		} else if r == chartHeight/2 {
			label = midLabel
		}

		b.WriteString(fmt.Sprintf("  %*s ", axisWidth, label))
		if label != "" {
			b.WriteString("┤")
		} else {
			b.WriteString("│")
		}

		rowBottom := (chartHeight - 1 - r) * subRows

		if symbol == GraphBraille {
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
		} else {
			for cc := 0; cc < charCols; cc++ {
				h := 0
				if cc < len(dotHeights) {
					h = dotHeights[cc]
				}
				fill := clampInt(h-rowBottom, 0, 8)
				if fill == 0 {
					b.WriteRune(' ')
				} else {
					b.WriteString(rowStyles[r].Render(vBlockChars[fill]))
				}
			}
		}
		b.WriteString("\n")
	}

	// X-axis.
	b.WriteString(fmt.Sprintf("  %*s └", axisWidth, ""))
	b.WriteString(strings.Repeat("─", charCols))
	b.WriteString("\n")

	// Month labels.
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

		charPos := i * slots / len(stats) / pointsPerCell
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

// resampleCounts maps stats onto exactly slots values so the chart fills its
// width. With fewer stats than slots, each stat spans a run of slots. With
// more, each slot holds the mean of the stats that fall into it.
func resampleCounts(stats []DayStat, slots int) []float64 {
	n := len(stats)
	if n == 0 || slots <= 0 {
		return nil
	}
	values := make([]float64, slots)
	if n <= slots {
		for i, s := range stats {
			for j := i * slots / n; j < (i+1)*slots/n; j++ {
				values[j] = float64(s.Count)
			}
		}
		return values
	}
	for j := range values {
		lo, hi := j*n/slots, (j+1)*n/slots
		sum := 0
		for _, s := range stats[lo:hi] {
			sum += s.Count
		}
		values[j] = float64(sum) / float64(hi-lo)
	}
	return values
}

// formatAxisValue formats a y-axis label, keeping one decimal for small
// fractional values such as averaged counts.
func formatAxisValue(v float64) string {
	if v == math.Trunc(v) || v >= 10 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

// pageStep returns how many rows a paging key moves a list that shows rows
// rows at a time: a full page for pgup/pgdown, half a page for ctrl+u/ctrl+d.
// It returns 0 for any other key.
func pageStep(key string, rows int) int {
	rows = max(rows, 1)
	switch key {
	case "pgdown":
		return rows
	case "pgup":
		return -rows
	case "ctrl+d":
		return max(rows/2, 1)
	case "ctrl+u":
		return -max(rows/2, 1)
	}
	return 0
}

// wheelRows is how many rows one notch of the mouse wheel scrolls.
const wheelRows = 3

// wheelStep returns how many rows a mouse wheel event scrolls a list, or 0 if
// msg is not a wheel event.
func wheelStep(msg tea.MouseMsg) int {
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		return wheelRows
	case tea.MouseButtonWheelUp:
		return -wheelRows
	}
	return 0
}
