package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type releasesView int

const (
	viewReleaseList releasesView = iota
	viewReleaseCadence
	releasesViewCount
)

var releasesViewNames = []string{"Timeline", "Cadence"}

type releasesPage struct {
	tags         []TagInfo
	commitsSince int
	view         releasesView
	offset       int
}

func newReleasesPage() *releasesPage { return &releasesPage{} }

func (p *releasesPage) Init() tea.Cmd { return nil }

func (p *releasesPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case tagsDataMsg:
		if msg.err == nil {
			p.tags = msg.tags
			p.commitsSince = msg.commitsSince
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			p.view = (p.view + 1) % releasesViewCount
			p.offset = 0
		case "j", "down":
			p.offset++
		case "k", "up":
			if p.offset > 0 {
				p.offset--
			}
		case "g":
			p.offset = 0
		case "G":
			if len(p.tags) > 0 {
				p.offset = len(p.tags) - 1
			}
		}
	}
	return p, nil
}

func (p *releasesPage) View(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	// View selector.
	var viewParts []string
	for i, name := range releasesViewNames {
		if releasesView(i) == p.view {
			viewParts = append(viewParts, boldStyle.Render(name))
		} else {
			viewParts = append(viewParts, dimStyle.Render(name))
		}
	}
	b.WriteString(fmt.Sprintf("  %s  %s", dimStyle.Render("[v]iew"), strings.Join(viewParts, dimStyle.Render(" / "))))
	b.WriteString("\n\n")

	if len(p.tags) == 0 {
		b.WriteString("  No tags found.")
		return b.String()
	}

	contentHeight := height - 3

	switch p.view {
	case viewReleaseList:
		b.WriteString(p.renderTimeline(width, contentHeight))
	case viewReleaseCadence:
		b.WriteString(p.renderCadence(width, contentHeight))
	}

	return b.String()
}

func (p *releasesPage) renderTimeline(width, height int) string {
	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}

	tags := p.tags
	if p.offset >= len(tags) {
		p.offset = len(tags) - 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := len(tags) - maxRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
	end := p.offset + maxRows
	if end > len(tags) {
		end = len(tags)
	}
	visible := tags[p.offset:end]

	if len(visible) == 0 {
		return "  No tags."
	}

	// Column widths.
	nameWidth := 0
	for _, t := range visible {
		if l := len(t.Name); l > nameWidth {
			nameWidth = l
		}
	}
	if nameWidth > width/3 {
		nameWidth = width / 3
	}

	dateWidth := 12 // "Jan 02, 2006"
	now := time.Now()

	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	currentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	commitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

	var b strings.Builder
	for i, t := range visible {
		globalIdx := p.offset + i

		name := t.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		// Marker for latest.
		style := tagStyle
		marker := "  "
		if globalIdx == 0 {
			style = currentStyle
			marker = "* "
		}

		date := t.Date.Format("Jan 02, 2006")
		age := formatRelativeDate(now, t.Date)

		// Days since previous release (next in the sorted list).
		gap := ""
		if globalIdx+1 < len(tags) {
			days := int(t.Date.Sub(tags[globalIdx+1].Date).Hours() / 24)
			if days > 0 {
				gap = fmt.Sprintf("%dd", days)
			}
		}

		// Hash preview.
		hash := t.CommitHash
		if len(hash) > 7 {
			hash = hash[:7]
		}

		b.WriteString(marker)
		b.WriteString(style.Render(fmt.Sprintf("%-*s", nameWidth, name)))
		b.WriteString("  ")
		b.WriteString(dimStyle.Render(fmt.Sprintf("%-*s", dateWidth, date)))
		b.WriteString("  ")
		b.WriteString(dimStyle.Render(fmt.Sprintf("%-9s", age)))

		if gap != "" {
			b.WriteString("  ")
			b.WriteString(commitStyle.Render(fmt.Sprintf("+%s", gap)))
		}

		if hash != "" {
			b.WriteString("  ")
			b.WriteString(dimStyle.Render(hash))
		}

		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d releases", len(tags))))
	if len(tags) > 1 {
		first := tags[len(tags)-1].Date
		last := tags[0].Date
		span := int(last.Sub(first).Hours() / 24)
		avg := span / (len(tags) - 1)
		b.WriteString(dimStyle.Render(fmt.Sprintf("  avg %dd apart", avg)))
	}

	return b.String()
}

func (p *releasesPage) renderCadence(width, height int) string {
	if len(p.tags) < 2 {
		return "  Need at least 2 releases for cadence chart."
	}

	// Compute gaps between consecutive releases (sorted newest first).
	var gaps []releaseGap
	for i := 0; i < len(p.tags)-1; i++ {
		days := int(p.tags[i].Date.Sub(p.tags[i+1].Date).Hours() / 24)
		gaps = append(gaps, releaseGap{
			from: p.tags[i+1].Name,
			to:   p.tags[i].Name,
			days: days,
		})
	}

	// For <10 tags, show the gap list. For >=10, show a distribution chart.
	if len(p.tags) < 10 {
		return p.renderGapList(gaps, width, height)
	}
	return p.renderGapDistribution(gaps, width, height)
}

func (p *releasesPage) renderGapList(gaps []releaseGap, width, height int) string {
	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if len(gaps) > maxRows {
		gaps = gaps[:maxRows]
	}

	maxDays := 0
	for _, g := range gaps {
		if g.days > maxDays {
			maxDays = g.days
		}
	}
	if maxDays == 0 {
		maxDays = 1
	}

	// Label width.
	labelWidth := 0
	for _, g := range gaps {
		l := len(g.from) + len(g.to) + 4 // " → "
		if l > labelWidth {
			labelWidth = l
		}
	}
	if labelWidth > width/2 {
		labelWidth = width / 2
	}

	daysWidth := len(fmt.Sprintf("%d days", maxDays))
	barMaxWidth := width - labelWidth - daysWidth - 8
	if barMaxWidth < 5 {
		barMaxWidth = 5
	}

	barGradient := []lipgloss.Color{
		lipgloss.Color("82"),
		lipgloss.Color("154"),
		lipgloss.Color("214"),
		lipgloss.Color("208"),
		lipgloss.Color("196"),
	}

	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	var b strings.Builder
	for _, g := range gaps {
		label := g.from + arrowStyle.Render(" → ") + g.to
		visualLabel := g.from + " → " + g.to
		if len(visualLabel) > labelWidth {
			// Truncate from name.
			avail := labelWidth - len(g.to) - 4
			if avail < 3 {
				avail = 3
			}
			from := g.from
			if len(from) > avail {
				from = from[:avail-1] + "…"
			}
			label = from + arrowStyle.Render(" → ") + g.to
			visualLabel = from + " → " + g.to
		}

		barLen := g.days * barMaxWidth / maxDays
		if barLen > barMaxWidth {
			barLen = barMaxWidth
		}

		ci := g.days * (len(barGradient) - 1) / maxDays
		if ci >= len(barGradient) {
			ci = len(barGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(barGradient[ci])

		pad := labelWidth - len(visualLabel)
		if pad < 0 {
			pad = 0
		}

		b.WriteString(fmt.Sprintf("  %s%s ", label, strings.Repeat(" ", pad)))
		if barLen > 0 {
			b.WriteString(barStyle.Render(strings.Repeat("█", barLen)))
		}
		barPad := barMaxWidth - barLen
		if barPad > 0 {
			b.WriteString(strings.Repeat(" ", barPad))
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %d days", g.days)))
		b.WriteString("\n")
	}

	return b.String()
}

type releaseGap struct {
	from, to string
	days     int
}

func (p *releasesPage) renderGapDistribution(gaps []releaseGap, width, height int) string {
	// Bucket gaps into ranges.
	type bucket struct {
		label string
		count int
		maxD  int
	}
	buckets := []bucket{
		{"< 1w", 0, 7},
		{"1-2w", 0, 14},
		{"2-4w", 0, 28},
		{"1-2mo", 0, 60},
		{"2-3mo", 0, 90},
		{"3-6mo", 0, 180},
		{"6-12mo", 0, 365},
		{"> 1y", 0, 99999},
	}

	for _, g := range gaps {
		for i := range buckets {
			if g.days <= buckets[i].maxD {
				buckets[i].count++
				break
			}
		}
	}

	// Remove empty trailing buckets.
	lastNonZero := 0
	for i, b := range buckets {
		if b.count > 0 {
			lastNonZero = i
		}
	}
	buckets = buckets[:lastNonZero+1]

	maxCount := 0
	for _, b := range buckets {
		if b.count > maxCount {
			maxCount = b.count
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	labelWidth := 7
	countWidth := len(fmt.Sprintf("%d", maxCount))
	if countWidth < 3 {
		countWidth = 3
	}
	barMaxWidth := width - labelWidth - countWidth - 10
	if barMaxWidth < 10 {
		barMaxWidth = 10
	}

	barGradient := []lipgloss.Color{
		lipgloss.Color("63"),
		lipgloss.Color("33"),
		lipgloss.Color("39"),
		lipgloss.Color("82"),
		lipgloss.Color("154"),
	}

	var b strings.Builder
	for _, bk := range buckets {
		barLen := bk.count * barMaxWidth / maxCount

		ci := bk.count * (len(barGradient) - 1) / maxCount
		if ci >= len(barGradient) {
			ci = len(barGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(barGradient[ci])

		b.WriteString(fmt.Sprintf("  %*s ", labelWidth, bk.label))
		if barLen > 0 {
			b.WriteString(barStyle.Render(strings.Repeat("█", barLen)))
		}
		pad := barMaxWidth - barLen
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf(" %*d", countWidth, bk.count)))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Stats.
	var daysList []int
	total := 0
	for _, g := range gaps {
		daysList = append(daysList, g.days)
		total += g.days
	}
	sort.Ints(daysList)
	median := daysList[len(daysList)/2]
	avg := total / len(daysList)
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d releases  avg %dd  median %dd  min %dd  max %dd",
		len(p.tags), avg, median, daysList[0], daysList[len(daysList)-1])))

	return b.String()
}
