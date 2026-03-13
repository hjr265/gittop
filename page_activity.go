package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type activityView int

const (
	viewHeatmap activityView = iota
	viewByHour
	viewByWeekday
	viewByMonth
	activityViewCount
)

var activityViewNames = []string{"Heatmap", "By Hour", "By Weekday", "By Month"}

type activityPage struct {
	stats   []DayStat
	commits []CommitInfo
	view    activityView
}

func newActivityPage() *activityPage { return &activityPage{} }

func (p *activityPage) Init() tea.Cmd { return nil }

func (p *activityPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case statsMsg:
		p.stats = msg.stats
	case commitsDataMsg:
		p.commits = msg.commits
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			p.view = (p.view + 1) % activityViewCount
		}
	}
	return p, nil
}

func (p *activityPage) View(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	// View selector.
	var viewParts []string
	for i, name := range activityViewNames {
		if activityView(i) == p.view {
			viewParts = append(viewParts, boldStyle.Render(name))
		} else {
			viewParts = append(viewParts, dimStyle.Render(name))
		}
	}
	b.WriteString(fmt.Sprintf("  %s  %s", dimStyle.Render("[v]iew"), strings.Join(viewParts, dimStyle.Render(" / "))))
	b.WriteString("\n\n")

	contentHeight := height - 3 // view selector + blank lines

	switch p.view {
	case viewHeatmap:
		b.WriteString(p.renderHeatmap(width, contentHeight))
	case viewByHour:
		b.WriteString(p.renderDistribution(width, contentHeight, "Hour of Day", 24, func(c *CommitInfo) int { return c.Hour }, func(i int) string { return fmt.Sprintf("%02d:00", i) }))
	case viewByWeekday:
		dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
		b.WriteString(p.renderDistribution(width, contentHeight, "Day of Week", 7, func(c *CommitInfo) int { return int(c.Weekday) }, func(i int) string { return dayNames[i] }))
	case viewByMonth:
		b.WriteString(p.renderDistribution(width, contentHeight, "Month", 12, func(c *CommitInfo) int { return int(c.Month) - 1 }, func(i int) string { return time.Month(i + 1).String()[:3] }))
	}

	return b.String()
}

func (p *activityPage) renderDistribution(width, height int, title string, bucketCount int, bucketFn func(*CommitInfo) int, labelFn func(int) string) string {
	if len(p.commits) == 0 {
		return "  No data."
	}

	// Count commits per bucket.
	counts := make([]int, bucketCount)
	for i := range p.commits {
		idx := bucketFn(&p.commits[i])
		if idx >= 0 && idx < bucketCount {
			counts[idx]++
		}
	}

	maxCount := 0
	total := 0
	for _, c := range counts {
		total += c
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	// Label width.
	labelWidth := 0
	for i := 0; i < bucketCount; i++ {
		if l := len(labelFn(i)); l > labelWidth {
			labelWidth = l
		}
	}

	// Count width for the number.
	countWidth := len(fmt.Sprintf("%d", maxCount))
	if countWidth < 3 {
		countWidth = 3
	}

	// Bar width: remaining space after label, count, margins.
	barMaxWidth := width - labelWidth - countWidth - 8 // "  label  ███ count"
	if barMaxWidth < 10 {
		barMaxWidth = 10
	}

	// Gradient colors for the bars.
	barGradient := []lipgloss.Color{
		lipgloss.Color("63"),
		lipgloss.Color("33"),
		lipgloss.Color("39"),
		lipgloss.Color("49"),
		lipgloss.Color("82"),
	}

	var b strings.Builder

	for i := 0; i < bucketCount; i++ {
		label := labelFn(i)
		count := counts[i]

		// Bar length proportional to max.
		barLen := count * barMaxWidth / maxCount

		// Pick color based on intensity.
		ci := count * (len(barGradient) - 1) / maxCount
		if ci >= len(barGradient) {
			ci = len(barGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(barGradient[ci])

		// Percentage.
		pct := float64(count) * 100 / float64(total)

		b.WriteString(fmt.Sprintf("  %*s ", labelWidth, label))

		if barLen > 0 {
			b.WriteString(barStyle.Render(strings.Repeat("█", barLen)))
		}

		// Pad to align count.
		pad := barMaxWidth - barLen
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}

		b.WriteString(dimStyle.Render(fmt.Sprintf(" %*d  %4.1f%%", countWidth, count, pct)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Total: %d commits", total)))

	return b.String()
}

func (p *activityPage) renderHeatmap(width, height int) string {
	if len(p.stats) == 0 {
		return "  No data."
	}

	var b strings.Builder

	countByDate := make(map[time.Time]int, len(p.stats))
	for _, s := range p.stats {
		countByDate[s.Date] = s.Count
	}

	cellWidth := 2
	labelWidth := 4
	margin := 4
	maxWeeks := (width - labelWidth - margin) / cellWidth
	if maxWeeks < 4 {
		maxWeeks = 4
	}
	if maxWeeks > 53 {
		maxWeeks = 53
	}

	today := truncateToDay(time.Now())

	endDay := today
	for endDay.Weekday() != time.Saturday {
		endDay = endDay.AddDate(0, 0, 1)
	}
	startDay := endDay.AddDate(0, 0, -(maxWeeks*7 - 1))

	maxCount := 0
	for d := startDay; !d.After(endDay); d = d.AddDate(0, 0, 1) {
		if c := countByDate[d]; c > maxCount {
			maxCount = c
		}
	}

	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("236"))
	levels := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("236")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("22")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("28")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("34")),
		lipgloss.NewStyle().Foreground(lipgloss.Color("46")),
	}

	cellBlock := "██"

	quantize := func(count int) int {
		if count == 0 || maxCount == 0 {
			return 0
		}
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

	grid := make([][]int, 7)
	dates := make([][]time.Time, 7)
	for i := range grid {
		grid[i] = make([]int, maxWeeks)
		dates[i] = make([]time.Time, maxWeeks)
	}

	for d := startDay; !d.After(endDay); d = d.AddDate(0, 0, 1) {
		if d.After(today) {
			continue
		}
		weekIdx := int(d.Sub(startDay).Hours()/24) / 7
		dayIdx := int(d.Weekday())
		if weekIdx >= 0 && weekIdx < maxWeeks {
			grid[dayIdx][weekIdx] = quantize(countByDate[d])
			dates[dayIdx][weekIdx] = d
		}
	}

	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	monthLabels := make([]byte, maxWeeks*cellWidth)
	for i := range monthLabels {
		monthLabels[i] = ' '
	}
	lastMonth := time.Month(0)
	for w := 0; w < maxWeeks; w++ {
		d := startDay.AddDate(0, 0, w*7+1)
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

	displayOrder := []int{1, 2, 3, 4, 5, 6, 0}

	for _, dayIdx := range displayOrder {
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
