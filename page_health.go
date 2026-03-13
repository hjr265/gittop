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
	viewMostAuthors
	viewStalestFiles
	healthViewCount
)

var healthViewNames = []string{"Largest Files", "Most Authors", "Stalest Files"}

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
		filterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
		b.WriteString(dimStyle.Render("    [f]ilter: "))
		b.WriteString(filterStyle.Render(p.pathInput))
		b.WriteString(filterStyle.Render("_"))
	} else if p.pathPattern != "" {
		filterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
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
	case viewMostAuthors:
		b.WriteString(p.renderMostAuthors(width, contentHeight))
	case viewStalestFiles:
		b.WriteString(p.renderStalestFiles(width, contentHeight))
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

	barGradient := []lipgloss.Color{
		lipgloss.Color("22"),
		lipgloss.Color("28"),
		lipgloss.Color("172"),
		lipgloss.Color("208"),
		lipgloss.Color("196"),
	}

	var b strings.Builder
	for i := range files {
		f := &files[i]
		val := valueFn(f)

		barLen := val * barMaxWidth / maxVal
		if barLen > barMaxWidth {
			barLen = barMaxWidth
		}

		ci := val * (len(barGradient) - 1) / maxVal
		if ci >= len(barGradient) {
			ci = len(barGradient) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(barGradient[ci])

		path := f.Path
		if len(path) > pathWidth {
			path = "..." + path[len(path)-pathWidth+3:]
		}

		b.WriteString(fmt.Sprintf("  %-*s ", pathWidth, path))

		if barLen > 0 {
			b.WriteString(barStyle.Render(strings.Repeat("█", barLen)))
		}

		pad := barMaxWidth - barLen
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}

		b.WriteString(dimStyle.Render(fmt.Sprintf(" %*s", labelWidth, labelFn(f))))
		b.WriteString("\n")
	}

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
