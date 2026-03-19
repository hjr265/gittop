package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type healthView int

const (
	viewLargestFiles healthView = iota
	viewMostChurn
	viewMostAuthors
	viewStalestFiles
	viewLanguages
	healthViewCount
)

var healthViewNames = []string{"Largest Files", "Most Churn", "Most Authors", "Stalest Files", "Languages"}

// healthTreeMsg carries cached line counts from the HEAD tree.
type healthTreeMsg struct {
	lineCounts map[string]int
	err        error
}

type healthPage struct {
	lineCounts map[string]int   // cached from tree, filter-independent
	commits    []CommitInfo     // filtered commits (with files)
	filtered   bool             // true when a global filter is active
	files      []FileHealthInfo // recomputed from lineCounts + commits
	view       healthView

	// Local file pattern filter.
	pathPattern string // glob pattern, e.g. "*.go"
	pathInput   string // current input while editing
	editing     bool   // true when typing pattern
}

func newHealthPage() *healthPage { return &healthPage{} }

func (p *healthPage) Init() tea.Cmd { return nil }

func (p *healthPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	switch msg := msg.(type) {
	case healthTreeMsg:
		if msg.err == nil {
			p.lineCounts = msg.lineCounts
			p.recompute()
		}
	case commitsDataMsg:
		p.commits = msg.commits
		p.filtered = msg.filtered
		p.recompute()
	case tea.KeyMsg:
		if p.editing {
			switch msg.String() {
			case "enter":
				p.pathPattern = p.pathInput
				p.editing = false
				p.recompute()
			case "esc":
				p.editing = false
				p.pathInput = p.pathPattern
			case "backspace":
				if len(p.pathInput) > 0 {
					p.pathInput = p.pathInput[:len(p.pathInput)-1]
				}
			default:
				if len(msg.String()) == 1 {
					p.pathInput += msg.String()
				}
			}
			return p, nil
		}
		switch msg.String() {
		case "v":
			p.view = (p.view + 1) % healthViewCount
		case "f":
			p.editing = true
			p.pathInput = p.pathPattern
		case "esc":
			if p.pathPattern != "" {
				p.pathPattern = ""
				p.pathInput = ""
				p.recompute()
			}
		}
	}
	return p, nil
}

func (p *healthPage) recompute() {
	if p.lineCounts == nil {
		return
	}
	all := BuildHealthData(p.lineCounts, p.commits, p.filtered)
	if p.pathPattern != "" {
		p.files = filterFilesByPath(all, p.pathPattern)
	} else {
		p.files = all
	}
}

func filterFilesByPath(files []FileHealthInfo, pattern string) []FileHealthInfo {
	pattern = strings.ToLower(pattern)
	var result []FileHealthInfo
	for _, f := range files {
		path := strings.ToLower(f.Path)
		// Match against full path and basename.
		if matched, _ := filepath.Match(pattern, path); matched {
			result = append(result, f)
			continue
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			result = append(result, f)
			continue
		}
		// Substring match as fallback.
		if strings.Contains(path, pattern) {
			result = append(result, f)
		}
	}
	return result
}

func (p *healthPage) View(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")

	// View selector + file filter indicator.
	var viewParts []string
	for i, name := range healthViewNames {
		if healthView(i) == p.view {
			viewParts = append(viewParts, boldStyle.Render(name))
		} else {
			viewParts = append(viewParts, dimStyle.Render(name))
		}
	}
	b.WriteString(fmt.Sprintf("  %s  %s", dimStyle.Render("[v]iew"), strings.Join(viewParts, dimStyle.Render(" / "))))

	if p.editing {
		filterStyle := lipgloss.NewStyle().Foreground(tagColor).Bold(true)
		b.WriteString(dimStyle.Render("    [f]ilter: "))
		b.WriteString(filterStyle.Render(p.pathInput))
		b.WriteString(filterStyle.Render("_"))
	} else if p.pathPattern != "" {
		filterStyle := lipgloss.NewStyle().Foreground(tagColor).Bold(true)
		b.WriteString(dimStyle.Render("    [f]ilter: "))
		b.WriteString(filterStyle.Render(p.pathPattern))
	}
	b.WriteString("\n\n")

	if p.lineCounts == nil {
		b.WriteString("  Loading health data...")
		return b.String()
	}

	contentHeight := height - 3 // view selector + blank lines

	switch p.view {
	case viewLargestFiles:
		b.WriteString(p.renderLargestFiles(width, contentHeight))
	case viewMostChurn:
		b.WriteString(p.renderMostChurn(width, contentHeight))
	case viewMostAuthors:
		b.WriteString(p.renderMostAuthors(width, contentHeight))
	case viewStalestFiles:
		b.WriteString(p.renderStalestFiles(width, contentHeight))
	case viewLanguages:
		b.WriteString(p.renderLanguages(width, contentHeight))
	}

	return b.String()
}

func (p *healthPage) renderLargestFiles(width, height int) string {
	sorted := make([]FileHealthInfo, len(p.files))
	copy(sorted, p.files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Lines > sorted[j].Lines })

	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if len(sorted) > maxRows {
		sorted = sorted[:maxRows]
	}

	if len(sorted) == 0 {
		return "  No data."
	}

	maxLines := sorted[0].Lines
	if maxLines == 0 {
		maxLines = 1
	}

	return p.renderFileList(sorted, width, func(f *FileHealthInfo) int { return f.Lines }, func(f *FileHealthInfo) string { return fmt.Sprintf("%d lines", f.Lines) }, maxLines)
}

func (p *healthPage) renderMostChurn(width, height int) string {
	var candidates []FileHealthInfo
	for _, f := range p.files {
		if f.Churn > 0 {
			candidates = append(candidates, f)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Churn > candidates[j].Churn })

	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if len(candidates) > maxRows {
		candidates = candidates[:maxRows]
	}

	if len(candidates) == 0 {
		return "  No data."
	}

	maxVal := candidates[0].Churn
	if maxVal == 0 {
		maxVal = 1
	}

	return p.renderFileList(candidates, width, func(f *FileHealthInfo) int { return f.Churn }, func(f *FileHealthInfo) string {
		return fmt.Sprintf("%d commits", f.Churn)
	}, maxVal)
}

func (p *healthPage) renderMostAuthors(width, height int) string {
	var candidates []FileHealthInfo
	for _, f := range p.files {
		if f.AuthorCount > 0 {
			candidates = append(candidates, f)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].AuthorCount > candidates[j].AuthorCount })

	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if len(candidates) > maxRows {
		candidates = candidates[:maxRows]
	}

	if len(candidates) == 0 {
		return "  No data."
	}

	maxVal := candidates[0].AuthorCount
	if maxVal == 0 {
		maxVal = 1
	}

	return p.renderFileList(candidates, width, func(f *FileHealthInfo) int { return f.AuthorCount }, func(f *FileHealthInfo) string {
		return fmt.Sprintf("%d authors", f.AuthorCount)
	}, maxVal)
}

func (p *healthPage) renderStalestFiles(width, height int) string {
	now := time.Now()
	var candidates []FileHealthInfo
	for _, f := range p.files {
		if !f.LastChanged.IsZero() {
			candidates = append(candidates, f)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastChanged.Before(candidates[j].LastChanged)
	})

	maxRows := height - 2
	if maxRows < 1 {
		maxRows = 1
	}
	if len(candidates) > maxRows {
		candidates = candidates[:maxRows]
	}

	if len(candidates) == 0 {
		return "  No data."
	}

	maxDays := int(now.Sub(candidates[0].LastChanged).Hours() / 24)
	if maxDays == 0 {
		maxDays = 1
	}

	return p.renderFileList(candidates, width, func(f *FileHealthInfo) int {
		return int(now.Sub(f.LastChanged).Hours() / 24)
	}, func(f *FileHealthInfo) string {
		return formatAge(now, f.LastChanged)
	}, maxDays)
}

func (p *healthPage) renderFileList(files []FileHealthInfo, width int, valueFn func(*FileHealthInfo) int, labelFn func(*FileHealthInfo) string, maxVal int) string {
	// Determine max path width.
	pathWidth := 0
	for i := range files {
		if l := len(files[i].Path); l > pathWidth {
			pathWidth = l
		}
	}
	if pathWidth > width/2 {
		pathWidth = width / 2
	}

	// Label width for the right side.
	labelWidth := 0
	for i := range files {
		if l := len(labelFn(&files[i])); l > labelWidth {
			labelWidth = l
		}
	}

	barMaxWidth := width - pathWidth - labelWidth - 8
	if barMaxWidth < 5 {
		barMaxWidth = 5
	}

	healthBarGradient := healthGradient

	var b strings.Builder
	for i := range files {
		f := &files[i]
		val := valueFn(f)

		ci := val * (len(healthBarGradient) - 1) / maxVal
		if ci >= len(healthBarGradient) {
			ci = len(healthBarGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(healthBarGradient[ci])

		path := f.Path
		if len(path) > pathWidth {
			path = "..." + path[len(path)-pathWidth+3:]
		}

		b.WriteString(fmt.Sprintf("  %-*s ", pathWidth, path))

		bar, barW := smoothBar(val, maxVal, barMaxWidth, barStyle)
		b.WriteString(bar)

		pad := barMaxWidth - barW
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}

		b.WriteString(dimStyle.Render(fmt.Sprintf(" %*s", labelWidth, labelFn(f))))
		b.WriteString("\n")
	}

	return b.String()
}

type langStat struct {
	ext   string
	lines int
	files int
}

func (p *healthPage) renderLanguages(width, height int) string {
	if p.lineCounts == nil {
		return "  No data."
	}

	// Group line counts by extension.
	extLines := map[string]int{}
	extFiles := map[string]int{}
	for path, lines := range p.lineCounts {
		// Apply path filter if active.
		if p.pathPattern != "" {
			low := strings.ToLower(path)
			pat := strings.ToLower(p.pathPattern)
			m1, _ := filepath.Match(pat, low)
			m2, _ := filepath.Match(pat, filepath.Base(low))
			if !m1 && !m2 && !strings.Contains(low, pat) {
				continue
			}
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			ext = "(no ext)"
		}
		extLines[ext] += lines
		extFiles[ext]++
	}

	if len(extLines) == 0 {
		return "  No data."
	}

	// Build sorted list.
	langs := make([]langStat, 0, len(extLines))
	totalLines := 0
	for ext, lines := range extLines {
		langs = append(langs, langStat{ext: ext, lines: lines, files: extFiles[ext]})
		totalLines += lines
	}
	sort.Slice(langs, func(i, j int) bool { return langs[i].lines > langs[j].lines })

	maxRows := height - 3
	if maxRows < 1 {
		maxRows = 1
	}
	if len(langs) > maxRows {
		langs = langs[:maxRows]
	}

	maxLines := langs[0].lines
	if maxLines == 0 {
		maxLines = 1
	}

	// Label widths.
	extWidth := 0
	for _, l := range langs {
		if len(l.ext) > extWidth {
			extWidth = len(l.ext)
		}
	}
	if extWidth < 6 {
		extWidth = 6
	}

	langGradient := chartGradient
	if len(langGradient) > 5 {
		langGradient = langGradient[:5]
	}

	// "  ext  ███ 12345 lines  45 files  12.3%"
	barMaxWidth := width - extWidth - 40
	if barMaxWidth < 10 {
		barMaxWidth = 10
	}

	var b strings.Builder
	for _, l := range langs {
		ci := l.lines * (len(langGradient) - 1) / maxLines
		if ci >= len(langGradient) {
			ci = len(langGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(langGradient[ci])

		pct := float64(l.lines) * 100 / float64(totalLines)

		b.WriteString(fmt.Sprintf("  %-*s ", extWidth, l.ext))

		bar, barW := smoothBar(l.lines, maxLines, barMaxWidth, barStyle)
		b.WriteString(bar)

		pad := barMaxWidth - barW
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}

		b.WriteString(dimStyle.Render(fmt.Sprintf(" %6d lines  %4d files  %5.1f%%", l.lines, l.files, pct)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d extensions, %d total lines", len(extLines), totalLines)))

	return b.String()
}

func formatAge(now time.Time, t time.Time) string {
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "1 day ago"
	case days < 30:
		return fmt.Sprintf("%d days ago", days)
	case days < 365:
		months := days / 30
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := days / 365
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}
