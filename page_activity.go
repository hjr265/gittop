package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type activityPage struct {
	stats []DayStat
}

func newActivityPage() *activityPage { return &activityPage{} }

func (p *activityPage) Init() tea.Cmd { return nil }

func (p *activityPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if msg, ok := msg.(statsMsg); ok {
		p.stats = msg.stats
	}
	return p, nil
}

func (p *activityPage) View(width, height int) string {
	if len(p.stats) == 0 {
		return "\n  No data."
	}

	var b strings.Builder
	b.WriteString("\n")

	// Build a date->count lookup.
	countByDate := make(map[time.Time]int, len(p.stats))
	for _, s := range p.stats {
		countByDate[s.Date] = s.Count
	}

	// Heatmap: rows = weekdays (Mon..Sun), columns = weeks.
	// Work backwards from today to fill available width.
	// Each cell is 2 chars wide ("██" or "  ") for a square look.
	cellWidth := 2
	labelWidth := 4 // "Mon " etc.
	margin := 4     // left + right margins
	maxWeeks := (width - labelWidth - margin) / cellWidth
	if maxWeeks < 4 {
		maxWeeks = 4
	}
	if maxWeeks > 53 {
		maxWeeks = 53
	}

	today := truncateToDay(time.Now())

	// Find the Saturday that ends the current week (or today if Saturday).
	// Weeks run Sun..Sat to match GitHub.
	endDay := today
	for endDay.Weekday() != time.Saturday {
		endDay = endDay.AddDate(0, 0, 1)
	}
	startDay := endDay.AddDate(0, 0, -(maxWeeks*7 - 1))

	// Find max for color scaling.
	maxCount := 0
	for d := startDay; !d.After(endDay); d = d.AddDate(0, 0, 1) {
		if c := countByDate[d]; c > maxCount {
			maxCount = c
		}
	}

	// Color levels (5 levels: empty, low, med, med-high, high).
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	levels := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("236")), // 0 commits
		lipgloss.NewStyle().Foreground(lipgloss.Color("22")),  // low
		lipgloss.NewStyle().Foreground(lipgloss.Color("28")),  // med
		lipgloss.NewStyle().Foreground(lipgloss.Color("34")),  // med-high
		lipgloss.NewStyle().Foreground(lipgloss.Color("46")),  // high
	}

	cellBlock := "██"

	// Quantize a count to a level 0..4.
	quantize := func(count int) int {
		if count == 0 || maxCount == 0 {
			return 0
		}
		// Use quartile thresholds relative to max.
		ratio := float64(count) / float64(maxCount)
		switch {
		case ratio <= 0.25:
			return 1
		case ratio <= 0.50:
			return 2
		case ratio <= 0.75:
			return 3
		default:
			return 4
		}
	}

	// Build the grid: 7 rows (Sun=0, Mon=1, ..., Sat=6), maxWeeks columns.
	grid := make([][]int, 7)       // grid[weekday][week] = level
	dates := make([][]time.Time, 7) // corresponding dates
	for i := range grid {
		grid[i] = make([]int, maxWeeks)
		dates[i] = make([]time.Time, maxWeeks)
	}

	for d := startDay; !d.After(endDay); d = d.AddDate(0, 0, 1) {
		if d.After(today) {
			continue
		}
		weekIdx := int(d.Sub(startDay).Hours()/24) / 7
		dayIdx := int(d.Weekday()) // Sun=0 .. Sat=6
		if weekIdx >= 0 && weekIdx < maxWeeks {
			grid[dayIdx][weekIdx] = quantize(countByDate[d])
			dates[dayIdx][weekIdx] = d
		}
	}

	// Title.
	b.WriteString(dimStyle.Render("  Contribution activity"))
	b.WriteString("\n\n")

	// Render month labels row.
	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	monthLabels := make([]byte, maxWeeks*cellWidth)
	for i := range monthLabels {
		monthLabels[i] = ' '
	}
	// Place month labels at the first week of each month.
	lastMonth := time.Month(0)
	for w := 0; w < maxWeeks; w++ {
		// Use Monday of this week for the label.
		d := startDay.AddDate(0, 0, w*7+1) // Monday
		if d.Month() != lastMonth {
			label := d.Format("Jan")
			pos := w * cellWidth
			for j := 0; j < len(label) && pos+j < len(monthLabels); j++ {
				monthLabels[pos+j] = label[j]
			}
			lastMonth = d.Month()
		}
	}
	b.WriteString(fmt.Sprintf("  %*s %s", labelWidth, "", dimStyle.Render(string(monthLabels))))
	b.WriteString("\n")

	// Render rows: Mon, Tue, Wed, Thu, Fri, Sat, Sun.
	// Display order: Mon(1), Tue(2), Wed(3), Thu(4), Fri(5), Sat(6), Sun(0)
	displayOrder := []int{1, 2, 3, 4, 5, 6, 0}

	for _, dayIdx := range displayOrder {
		// Show label only on Mon, Wed, Fri for compactness.
		label := ""
		if dayIdx == 1 || dayIdx == 3 || dayIdx == 5 {
			label = dayNames[dayIdx]
		}
		b.WriteString(fmt.Sprintf("  %*s ", labelWidth, label))

		for w := 0; w < maxWeeks; w++ {
			d := dates[dayIdx][w]
			if d.IsZero() || d.After(today) {
				b.WriteString(emptyStyle.Render(cellBlock))
			} else {
				lvl := grid[dayIdx][w]
				b.WriteString(levels[lvl].Render(cellBlock))
			}
		}
		b.WriteString("\n")
	}

	// Legend.
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %*s ", labelWidth, ""))
	b.WriteString(dimStyle.Render("Less "))
	for _, style := range levels {
		b.WriteString(style.Render(cellBlock))
	}
	b.WriteString(dimStyle.Render(" More"))

	if maxCount > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("    Max: %d commits/day", maxCount)))
	}

	return b.String()
}
