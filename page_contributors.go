package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type authorFileCount struct {
	Path    string
	Commits int
}

type authorInfo struct {
	Name        string
	Commits     int
	FirstCommit time.Time
	LastCommit  time.Time
	ActiveDays  int
	ActiveWeeks int
	TotalWeeks  int
	OwnedFiles  int

	// Detail data computed per-author.
	HourDist    [24]int
	WeekdayDist [7]int
	TopFiles    []authorFileCount
	WeeklyData  []int // commits per calendar week, oldest first
}

type contributorsView int

const (
	viewRankedList contributorsView = iota
	viewActivityLine
	contributorsViewCount
)

var contributorsViewNames = []string{"Ranked", "Activity"}

type contributorsPage struct {
	commits     []CommitInfo
	authors     []authorInfo
	cursor      int
	offset      int // scroll offset for left panel
	needFiles   bool
	graphSymbol GraphSymbol
	view        contributorsView
}

func newContributorsPage() *contributorsPage { return &contributorsPage{} }

func (p *contributorsPage) Init() tea.Cmd { return nil }

func (p *contributorsPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case commitsDataMsg:
		p.commits = msg.commits
		p.recompute()
	case graphSymbolMsg:
		p.graphSymbol = msg.symbol
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			p.view = (p.view + 1) % contributorsViewCount
		case "j", "down":
			if p.cursor < len(p.authors)-1 {
				p.cursor++
			}
		case "k", "up":
			if p.cursor > 0 {
				p.cursor--
			}
		case "g":
			p.cursor = 0
			p.offset = 0
		case "G":
			if len(p.authors) > 0 {
				p.cursor = len(p.authors) - 1
			}
		}
	}
	return p, nil
}

func (p *contributorsPage) recompute() {
	if len(p.commits) == 0 {
		p.authors = nil
		return
	}

	type authorData struct {
		name        string
		commits     int
		first, last time.Time
		daySet      map[time.Time]bool
		weekSet     map[string]bool
		fileCounts  map[string]int
		hourDist    [24]int
		weekdayDist [7]int
		weeklyMap   map[string]int // "2006-W02" -> count
	}

	byAuthor := map[string]*authorData{}
	hasFiles := false

	for i := range p.commits {
		c := &p.commits[i]
		key := c.Author
		ad, ok := byAuthor[key]
		if !ok {
			ad = &authorData{
				name:       c.Author,
				first:      c.Date,
				last:       c.Date,
				daySet:     map[time.Time]bool{},
				weekSet:    map[string]bool{},
				fileCounts: map[string]int{},
				weeklyMap:  map[string]int{},
			}
			byAuthor[key] = ad
		}
		ad.commits++
		if c.Date.Before(ad.first) {
			ad.first = c.Date
		}
		if c.Date.After(ad.last) {
			ad.last = c.Date
		}
		ad.daySet[c.Date] = true
		y, w := c.Date.ISOWeek()
		wk := fmt.Sprintf("%d-W%02d", y, w)
		ad.weekSet[wk] = true
		ad.weeklyMap[wk]++
		ad.hourDist[c.Hour]++
		ad.weekdayDist[c.Weekday]++

		if len(c.Files) > 0 {
			hasFiles = true
			for _, f := range c.Files {
				ad.fileCounts[f]++
			}
		}
	}

	// Compute file ownership.
	ownerCounts := map[string]int{}
	if hasFiles {
		fileAuthors := map[string]map[string]int{}
		for authorName, ad := range byAuthor {
			for file, count := range ad.fileCounts {
				if fileAuthors[file] == nil {
					fileAuthors[file] = map[string]int{}
				}
				fileAuthors[file][authorName] += count
			}
		}
		for _, authors := range fileAuthors {
			bestAuthor := ""
			bestCount := 0
			for a, c := range authors {
				if c > bestCount {
					bestCount = c
					bestAuthor = a
				}
			}
			if bestAuthor != "" {
				ownerCounts[bestAuthor]++
			}
		}
	}

	p.needFiles = !hasFiles
	var authors []authorInfo
	for _, ad := range byAuthor {
		totalWeeks := int(ad.last.Sub(ad.first).Hours()/24/7) + 1
		if totalWeeks < 1 {
			totalWeeks = 1
		}

		// Build weekly time series.
		weeklyData := buildWeeklySeries(ad.first, ad.last, ad.weeklyMap)

		// Build top files.
		var topFiles []authorFileCount
		for path, count := range ad.fileCounts {
			topFiles = append(topFiles, authorFileCount{Path: path, Commits: count})
		}
		sort.Slice(topFiles, func(i, j int) bool { return topFiles[i].Commits > topFiles[j].Commits })
		if len(topFiles) > 8 {
			topFiles = topFiles[:8]
		}

		authors = append(authors, authorInfo{
			Name:        ad.name,
			Commits:     ad.commits,
			FirstCommit: ad.first,
			LastCommit:  ad.last,
			ActiveDays:  len(ad.daySet),
			ActiveWeeks: len(ad.weekSet),
			TotalWeeks:  totalWeeks,
			OwnedFiles:  ownerCounts[ad.name],
			HourDist:    ad.hourDist,
			WeekdayDist: ad.weekdayDist,
			TopFiles:    topFiles,
			WeeklyData:  weeklyData,
		})
	}
	sort.Slice(authors, func(i, j int) bool { return authors[i].Commits > authors[j].Commits })
	p.authors = authors

	if p.cursor >= len(p.authors) {
		p.cursor = 0
		p.offset = 0
	}
}

// buildWeeklySeries returns a slice of commit counts per ISO week from first to last.
func buildWeeklySeries(first, last time.Time, weeklyMap map[string]int) []int {
	if first.IsZero() || last.IsZero() {
		return nil
	}
	var result []int
	d := first
	for !d.After(last) {
		y, w := d.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", y, w)
		result = append(result, weeklyMap[key])
		d = d.AddDate(0, 0, 7)
	}
	return result
}

func (p *contributorsPage) View(width, height int) string {
	if len(p.authors) == 0 {
		return "\n  No data."
	}

	contentHeight := height - 1

	// Split: left panel ~38%, right panel ~62%.
	leftWidth := width * 38 / 100
	if leftWidth < 25 {
		leftWidth = 25
	}
	if leftWidth > width-30 {
		leftWidth = width - 30
	}
	rightWidth := width - leftWidth - 1 // 1 for separator

	var leftPanel string
	switch p.view {
	case viewActivityLine:
		leftPanel = p.renderActivityLinePanel(leftWidth, contentHeight)
	default:
		leftPanel = p.renderLeftPanel(leftWidth, contentHeight)
	}
	rightPanel := p.renderRightPanel(rightWidth, contentHeight)

	// Join panels with a vertical separator.
	sepStyle := lipgloss.NewStyle().Foreground(selectionBg)
	separator := strings.Repeat(sepStyle.Render("│")+"\n", contentHeight)

	leftStyled := lipgloss.NewStyle().Width(leftWidth).Height(contentHeight).Render(leftPanel)
	rightStyled := lipgloss.NewStyle().Width(rightWidth).Height(contentHeight).Render(rightPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, separator, rightStyled)
}

func (p *contributorsPage) renderViewHeader() string {
	var viewHeaderParts []string
	for i, name := range contributorsViewNames {
		if contributorsView(i) == p.view {
			viewHeaderParts = append(viewHeaderParts, boldStyle.Render(name))
		} else {
			viewHeaderParts = append(viewHeaderParts, dimStyle.Render(name))
		}
	}
	return fmt.Sprintf("  %s  %s", dimStyle.Render("[v]iew"), strings.Join(viewHeaderParts, dimStyle.Render(" / ")))
}

func (p *contributorsPage) renderLeftPanel(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	b.WriteString(p.renderViewHeader())
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d contributors", len(p.authors))))
	b.WriteString("\n\n")

	visibleRows := height - 5
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Ensure cursor visible.
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+visibleRows {
		p.offset = p.cursor - visibleRows + 1
	}
	maxOffset := len(p.authors) - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}

	end := p.offset + visibleRows
	if end > len(p.authors) {
		end = len(p.authors)
	}

	maxCommits := p.authors[0].Commits
	if maxCommits == 0 {
		maxCommits = 1
	}

	// Compute column widths for visible rows.
	nameWidth := 0
	for i := p.offset; i < end; i++ {
		if l := len(p.authors[i].Name); l > nameWidth {
			nameWidth = l
		}
	}
	maxNameWidth := width - 15 // room for bar + count
	if maxNameWidth < 8 {
		maxNameWidth = 8
	}
	if nameWidth > maxNameWidth {
		nameWidth = maxNameWidth
	}

	countWidth := len(fmt.Sprintf("%d", maxCommits))
	barMaxWidth := width - nameWidth - countWidth - 6
	if barMaxWidth < 3 {
		barMaxWidth = 3
	}

	contribBarGradient := barGradient

	selectedStyle := lipgloss.NewStyle().Background(selectionBg)

	for i := p.offset; i < end; i++ {
		a := &p.authors[i]
		name := a.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		ci := a.Commits * (len(contribBarGradient) - 1) / maxCommits
		if ci >= len(contribBarGradient) {
			ci = len(contribBarGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(contribBarGradient[ci])

		marker := " "
		if i == p.cursor {
			marker = "▸"
		}

		bar, barW := smoothBar(a.Commits, maxCommits, barMaxWidth, barStyle)
		pad := barMaxWidth - barW
		padStr := ""
		if pad > 0 {
			padStr = strings.Repeat(" ", pad)
		}

		line := fmt.Sprintf(" %s %-*s %s%s %*d",
			marker, nameWidth, name, bar, padStr, countWidth, a.Commits)

		if i == p.cursor {
			// Pad to full width for highlight.
			visual := lipgloss.Width(line)
			if visual < width {
				line += strings.Repeat(" ", width-visual)
			}
			line = selectedStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (p *contributorsPage) renderActivityLinePanel(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	b.WriteString(p.renderViewHeader())
	b.WriteString("\n\n")

	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d contributors", len(p.authors))))
	b.WriteString("\n\n")

	visibleRows := height - 4
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Ensure cursor visible.
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+visibleRows {
		p.offset = p.cursor - visibleRows + 1
	}
	maxOffset := len(p.authors) - visibleRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}

	end := p.offset + visibleRows
	if end > len(p.authors) {
		end = len(p.authors)
	}

	// Determine the global time range across all visible authors.
	var globalFirst, globalLast time.Time
	for _, a := range p.authors {
		if globalFirst.IsZero() || a.FirstCommit.Before(globalFirst) {
			globalFirst = a.FirstCommit
		}
		if globalLast.IsZero() || a.LastCommit.After(globalLast) {
			globalLast = a.LastCommit
		}
	}
	now := time.Now()
	if now.After(globalLast) {
		globalLast = now
	}
	globalSpan := globalLast.Sub(globalFirst)
	if globalSpan <= 0 {
		globalSpan = 24 * time.Hour
	}

	// Compute column widths.
	nameWidth := 0
	for i := p.offset; i < end; i++ {
		if l := len(p.authors[i].Name); l > nameWidth {
			nameWidth = l
		}
	}
	maxNameWidth := width/3 - 4
	if maxNameWidth < 8 {
		maxNameWidth = 8
	}
	if nameWidth > maxNameWidth {
		nameWidth = maxNameWidth
	}

	lineWidth := width - nameWidth - 5 // marker + spaces + name + space
	if lineWidth < 5 {
		lineWidth = 5
	}

	selectedStyle := lipgloss.NewStyle().Background(selectionBg)
	activeColor := lipgloss.NewStyle().Foreground(activeLineColor)
	dormantColor := lipgloss.NewStyle().Foreground(dormantLineColor)


	const dormantThreshold = 90 * 24 * time.Hour

	// Precompute per-author bucket hits for the visible authors.
	// Each bucket corresponds to one column of the activity line.
	type authorBuckets struct {
		hits       []bool      // true if author committed in this bucket
		lastBefore []time.Time // most recent commit date on or before bucket start
	}
	bucketData := make(map[string]*authorBuckets)
	for i := p.offset; i < end; i++ {
		ab := &authorBuckets{
			hits:       make([]bool, lineWidth),
			lastBefore: make([]time.Time, lineWidth),
		}
		bucketData[p.authors[i].Name] = ab
		// Initialize lastBefore with the author's first commit.
		for col := range ab.lastBefore {
			ab.lastBefore[col] = p.authors[i].FirstCommit
		}
	}
	for ci := range p.commits {
		c := &p.commits[ci]
		ab, ok := bucketData[c.Author]
		if !ok {
			continue
		}
		col := int(c.Date.Sub(globalFirst) * time.Duration(lineWidth) / globalSpan)
		if col >= lineWidth {
			col = lineWidth - 1
		}
		if col < 0 {
			col = 0
		}
		ab.hits[col] = true
		if c.Date.After(ab.lastBefore[col]) {
			ab.lastBefore[col] = c.Date
		}
	}
	// Forward-propagate lastBefore so each bucket knows the most recent commit at or before it.
	for _, ab := range bucketData {
		for col := 1; col < lineWidth; col++ {
			if ab.lastBefore[col-1].After(ab.lastBefore[col]) {
				ab.lastBefore[col] = ab.lastBefore[col-1]
			}
		}
	}

	for i := p.offset; i < end; i++ {
		a := &p.authors[i]
		name := a.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "…"
		}

		marker := " "
		if i == p.cursor {
			marker = "▸"
		}

		ab := bucketData[a.Name]

		// Build per-column activity line.
		var lineBuf strings.Builder
		for col := 0; col < lineWidth; col++ {
			t := globalFirst.Add(globalSpan * time.Duration(col) / time.Duration(lineWidth))

			if t.Before(a.FirstCommit) || t.After(a.LastCommit) {
				lineBuf.WriteRune(' ')
				continue
			}

			if ab.hits[col] {
				lineBuf.WriteString(activeColor.Render("━"))
			} else {
				if t.Sub(ab.lastBefore[col]) > dormantThreshold {
					lineBuf.WriteString(dormantColor.Render("━"))
				} else {
					lineBuf.WriteString(activeColor.Render("─"))
				}
			}
		}

		line := fmt.Sprintf(" %s %-*s %s", marker, nameWidth, name, lineBuf.String())

		if i == p.cursor {
			visual := lipgloss.Width(line)
			if visual < width {
				line += strings.Repeat(" ", width-visual)
			}
			line = selectedStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (p *contributorsPage) renderRightPanel(width, height int) string {
	if p.cursor < 0 || p.cursor >= len(p.authors) {
		return ""
	}

	a := &p.authors[p.cursor]
	now := time.Now()

	var b strings.Builder
	b.WriteString("\n")

	// Header: name + rank.
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(brightColor)
	rankStyle := lipgloss.NewStyle().Foreground(mutedColor)
	b.WriteString(fmt.Sprintf(" %s  %s\n", nameStyle.Render(a.Name), rankStyle.Render(fmt.Sprintf("#%d", p.cursor+1))))

	statStyle := lipgloss.NewStyle().Foreground(positiveColor)
	b.WriteString(fmt.Sprintf(" %s commits · %s active days\n",
		statStyle.Render(fmt.Sprintf("%d", a.Commits)),
		statStyle.Render(fmt.Sprintf("%d", a.ActiveDays))))
	b.WriteString("\n")

	// Activity span.
	sectionStyle := lipgloss.NewStyle().Foreground(infoColor).Bold(true)
	b.WriteString(fmt.Sprintf(" %s\n", sectionStyle.Render("Activity")))

	spanDays := int(a.LastCommit.Sub(a.FirstCommit).Hours() / 24)
	b.WriteString(fmt.Sprintf(" %s → %s",
		dimStyle.Render(a.FirstCommit.Format("Jan 2006")),
		dimStyle.Render(a.LastCommit.Format("Jan 2006"))))
	b.WriteString(dimStyle.Render(fmt.Sprintf("  (%d days)", spanDays)))

	daysSinceLast := int(now.Sub(a.LastCommit).Hours() / 24)
	if daysSinceLast > 90 {
		dormantStyle := lipgloss.NewStyle().Foreground(errorColor)
		b.WriteString("  ")
		b.WriteString(dormantStyle.Render("dormant"))
	}
	b.WriteString("\n")

	// Cadence.
	consistency := float64(a.ActiveWeeks) / float64(a.TotalWeeks) * 100
	if a.TotalWeeks <= 1 {
		consistency = 100
	}
	var cadenceClr lipgloss.Color
	switch {
	case consistency >= 75:
		cadenceClr = positiveColor
	case consistency >= 50:
		cadenceClr = tagColor
	case consistency >= 25:
		cadenceClr = warningColor
	default:
		cadenceClr = errorColor
	}
	cadenceStyle := lipgloss.NewStyle().Foreground(cadenceClr)
	b.WriteString(fmt.Sprintf(" Cadence: %s consistency (%d/%d weeks)\n",
		cadenceStyle.Render(fmt.Sprintf("%.0f%%", consistency)),
		a.ActiveWeeks, a.TotalWeeks))
	b.WriteString("\n")

	linesUsed := 8 // lines rendered so far
	remainingHeight := height - linesUsed

	// Weekly activity mini-chart.
	if len(a.WeeklyData) > 1 && remainingHeight > 5 {
		b.WriteString(fmt.Sprintf(" %s\n", sectionStyle.Render("Weekly Activity")))

		chartWidth := width - 2
		if chartWidth > 60 {
			chartWidth = 60
		}
		chartHeight := 3
		if remainingHeight > 12 {
			chartHeight = 4
		}

		chartStr := renderMiniChart(a.WeeklyData, chartWidth, chartHeight, p.graphSymbol)
		for _, line := range strings.Split(chartStr, "\n") {
			if line != "" {
				b.WriteString(" ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		linesUsed += chartHeight + 3
		remainingHeight = height - linesUsed
	}

	// Schedule: weekday distribution.
	if remainingHeight > 5 {
		b.WriteString(fmt.Sprintf(" %s\n", sectionStyle.Render("Schedule")))

		dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
		maxDay := 0
		for _, c := range a.WeekdayDist {
			if c > maxDay {
				maxDay = c
			}
		}
		if maxDay == 0 {
			maxDay = 1
		}

		barWidth := width - 14 // " Day ████ NNN"
		if barWidth < 5 {
			barWidth = 5
		}

		dayStyle := lipgloss.NewStyle().Foreground(positiveColor)
		for d := 1; d <= 7; d++ {
			idx := d % 7 // Mon=1 → idx 1, Sun=0 → idx 0
			count := a.WeekdayDist[idx]
			bar, barW := smoothBar(count, maxDay, barWidth, dayStyle)
			pad := barWidth - barW
			padStr := ""
			if pad > 0 {
				padStr = strings.Repeat(" ", pad)
			}
			b.WriteString(fmt.Sprintf(" %s %s%s %s\n",
				dimStyle.Render(dayNames[idx]),
				bar, padStr,
				dimStyle.Render(fmt.Sprintf("%3d", count))))
		}

		// Peak hour.
		peakHour := 0
		peakCount := 0
		for h, c := range a.HourDist {
			if c > peakCount {
				peakCount = c
				peakHour = h
			}
		}
		b.WriteString(fmt.Sprintf(" Peak hour: %s\n",
			dimStyle.Render(fmt.Sprintf("%02d:00 (%d commits)", peakHour, peakCount))))
		b.WriteString("\n")
		linesUsed += 10
		remainingHeight = height - linesUsed
	}

	// Top files.
	if len(a.TopFiles) > 0 && remainingHeight > 3 {
		header := "Top Files"
		if a.OwnedFiles > 0 {
			header = fmt.Sprintf("Top Files (%d owned)", a.OwnedFiles)
		}
		b.WriteString(fmt.Sprintf(" %s\n", sectionStyle.Render(header)))

		maxShow := remainingHeight - 2
		if maxShow > len(a.TopFiles) {
			maxShow = len(a.TopFiles)
		}
		if maxShow < 1 {
			maxShow = 1
		}

		pathWidth := width - 12 // room for count
		if pathWidth < 10 {
			pathWidth = 10
		}

		fileStyle := lipgloss.NewStyle().Foreground(tagColor)
		for i := 0; i < maxShow; i++ {
			f := a.TopFiles[i]
			path := f.Path
			if len(path) > pathWidth {
				path = "…" + path[len(path)-pathWidth+1:]
			}
			b.WriteString(fmt.Sprintf(" %s %s\n",
				fileStyle.Render(fmt.Sprintf("%-*s", pathWidth, path)),
				dimStyle.Render(fmt.Sprintf("%3d", f.Commits))))
		}
	} else if p.needFiles && remainingHeight > 3 {
		b.WriteString(fmt.Sprintf(" %s\n", sectionStyle.Render("Top Files")))
		b.WriteString(dimStyle.Render(" Loading file data..."))
		b.WriteString("\n")
	}

	return b.String()
}

// renderMiniChart renders a small area chart without axes.
func renderMiniChart(values []int, width, height int, symbol GraphSymbol) string {
	if len(values) == 0 || width < 2 || height < 1 {
		return ""
	}

	maxVal := 0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		return ""
	}

	pointsPerCell := 1
	subRows := 8
	if symbol == GraphBraille {
		pointsPerCell = 2
		subRows = 4
	}

	maxData := width * pointsPerCell
	data := values
	if len(data) > maxData {
		data = data[len(data)-maxData:]
	}

	totalDotRows := height * subRows

	// Scale data.
	scaled := make([]int, len(data))
	for i, v := range data {
		scaled[i] = v * totalDotRows / maxVal
	}

	charCols := len(data)
	if symbol == GraphBraille {
		charCols = (len(data) + 1) / 2
	}
	chartStyle := lipgloss.NewStyle().Foreground(miniChartColor)

	var b strings.Builder
	for r := 0; r < height; r++ {
		rowBottom := (height - 1 - r) * subRows
		if symbol == GraphBraille {
			for cc := 0; cc < charCols && cc < width; cc++ {
				li := cc * 2
				ri := cc*2 + 1
				lh, rh := 0, 0
				if li < len(scaled) {
					lh = scaled[li]
				}
				if ri < len(scaled) {
					rh = scaled[ri]
				}
				ld := clampInt(lh-rowBottom, 0, 4)
				rd := clampInt(rh-rowBottom, 0, 4)
				if ld == 0 && rd == 0 {
					b.WriteRune(' ')
				} else {
					ch := rune(0x2800 + brailleLeftFill[ld] + brailleRightFill[rd])
					b.WriteString(chartStyle.Render(string(ch)))
				}
			}
		} else {
			for cc := 0; cc < charCols && cc < width; cc++ {
				h := 0
				if cc < len(scaled) {
					h = scaled[cc]
				}
				fill := clampInt(h-rowBottom, 0, 8)
				if fill == 0 {
					b.WriteRune(' ')
				} else {
					b.WriteString(chartStyle.Render(vBlockChars[fill]))
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatPct(pct float64) string {
	if pct >= 10 {
		return fmt.Sprintf("%.0f", pct)
	}
	if pct >= 1 {
		return fmt.Sprintf("%.1f", pct)
	}
	return fmt.Sprintf("%.1f", math.Max(pct, 0.1))
}
