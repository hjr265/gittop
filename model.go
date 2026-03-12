package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
)

const (
	chartPadding = 10 // left axis + right margin
	chartTopRows = 4  // title + subtitle + blank lines above chart
	chartBotRows = 4  // x-axis + month labels + footer + blank
	maxFetchDays = 365
)

var blocks = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

var gradient = []lipgloss.Color{
	lipgloss.Color("63"),
	lipgloss.Color("33"),
	lipgloss.Color("39"),
	lipgloss.Color("49"),
	lipgloss.Color("82"),
	lipgloss.Color("154"),
}

type model struct {
	repo     *git.Repository
	repoPath string
	stats    []DayStat
	width    int
	height   int
	err      error
	loading  bool
}

type statsMsg struct {
	stats []DayStat
	err   error
}

func newModel(repo *git.Repository, path string) model {
	return model{
		repo:     repo,
		repoPath: path,
		loading:  true,
	}
}

func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		stats, err := CollectDailyStats(m.repo, maxFetchDays)
		return statsMsg{stats: stats, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case statsMsg:
		m.stats = msg.stats
		m.err = msg.err
		m.loading = false
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.loading {
		return "\n  Scanning repository..."
	}
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n", m.err)
	}
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Determine how many columns (days) we can display.
	cols := m.width - chartPadding
	if cols < 10 {
		cols = 10
	}

	// Slice stats to fit available columns.
	stats := m.stats
	if len(stats) > cols {
		stats = stats[len(stats)-cols:]
	}

	// Chart height in rows.
	chartHeight := m.height - chartTopRows - chartBotRows
	if chartHeight < 3 {
		chartHeight = 3
	}
	if chartHeight > 24 {
		chartHeight = 24
	}

	// Find peak.
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
		maxCount = 1 // avoid division by zero
	}

	// Precompute units per column (resolution = chartHeight * 8).
	units := make([]int, len(stats))
	totalUnits := chartHeight * 8
	for i, s := range stats {
		units[i] = s.Count * totalUnits / maxCount
	}

	// Color styles per row.
	rowStyles := make([]lipgloss.Style, chartHeight)
	for r := 0; r < chartHeight; r++ {
		// r=0 is top (hot), r=chartHeight-1 is bottom (cool).
		ci := (chartHeight - 1 - r) * len(gradient) / chartHeight
		if ci >= len(gradient) {
			ci = len(gradient) - 1
		}
		rowStyles[r] = lipgloss.NewStyle().Foreground(gradient[ci])
	}

	// Axis label width.
	axisWidth := len(fmt.Sprintf("%d", maxCount))
	if axisWidth < 3 {
		axisWidth = 3
	}

	var b strings.Builder

	// Title.
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle := lipgloss.NewStyle().Faint(true)

	b.WriteString("\n")
	b.WriteString(titleStyle.Render(fmt.Sprintf("  gittop: %s", m.repoPath)))
	b.WriteString("\n")
	daysShown := len(stats)
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Commits over the last %d days", daysShown)))
	b.WriteString("\n\n")

	// Render chart rows from top to bottom.
	for r := 0; r < chartHeight; r++ {
		// Y-axis label: show on first, middle, and last row.
		label := ""
		rowBottom := (chartHeight - 1 - r) * maxCount / chartHeight
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

		// Each column.
		_ = rowBottom
		for c := 0; c < len(stats); c++ {
			u := units[c]
			// This row represents units from rowLow to rowHigh.
			rowLow := (chartHeight - 1 - r) * 8
			rowHigh := rowLow + 8

			var ch string
			if u >= rowHigh {
				ch = "█"
			} else if u > rowLow {
				ch = blocks[u-rowLow]
			} else {
				ch = " "
			}
			b.WriteString(rowStyles[r].Render(ch))
		}
		b.WriteString("\n")
	}

	// X-axis line.
	b.WriteString(fmt.Sprintf("  %*s └", axisWidth, ""))
	b.WriteString(strings.Repeat("─", len(stats)))
	b.WriteString("\n")

	// Month labels.
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
			pos := i
			for j := 0; j < len(label) && pos+j < len(monthLine); j++ {
				monthLine[pos+j] = label[j]
			}
		}
	}
	b.WriteString(fmt.Sprintf("  %*s  ", axisWidth, ""))
	b.WriteString(dimStyle.Render(string(monthLine)))
	b.WriteString("\n\n")

	// Footer summary.
	peakStr := ""
	if !peakDate.IsZero() {
		peakStr = fmt.Sprintf("  Peak: %d (%s)", maxCount, peakDate.Format("Jan 2"))
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("  Total: %d commits%s", totalCommits, peakStr)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Press q to quit"))
	b.WriteString("\n")

	return b.String()
}
