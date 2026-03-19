package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme captures all semantic color roles used throughout the TUI.
type Theme struct {
	Name string `toml:"-"`

	Chrome  ChromeColors `toml:"chrome"`
	Text    TextColors   `toml:"text"`
	Status  StatusColors `toml:"status"`
	Charts  ChartColors  `toml:"charts"`
	Diff    DiffColors   `toml:"diff"`
}

type ChromeColors struct {
	BarBg         string `toml:"bar_bg"`
	TabActiveBg   string `toml:"tab_active_bg"`
	TabActiveFg   string `toml:"tab_active_fg"`
	TabInactiveBg string `toml:"tab_inactive_bg"`
	ModalBg       string `toml:"modal_bg"`
	ModalBorder   string `toml:"modal_border"`
	SelectionBg   string `toml:"selection_bg"`
}

type TextColors struct {
	Bright string `toml:"bright"`
	Muted  string `toml:"muted"`
	Dim    string `toml:"dim"`
	Accent string `toml:"accent"`
	Zero   string `toml:"zero"`
}

type StatusColors struct {
	Positive string `toml:"positive"`
	Warning  string `toml:"warning"`
	Error    string `toml:"error"`
	Info     string `toml:"info"`
	Tag      string `toml:"tag"`
}

type ChartColors struct {
	Gradient        []string `toml:"gradient"`
	BarGradient     []string `toml:"bar_gradient"`
	HeatmapLevels   []string `toml:"heatmap_levels"`
	HealthGradient  []string `toml:"health_gradient"`
	CadenceGradient []string `toml:"cadence_gradient"`
	MiniChart       string   `toml:"mini_chart"`
	ActiveLine      string   `toml:"active_line"`
	DormantLine     string   `toml:"dormant_line"`
}

type DiffColors struct {
	Added      string `toml:"added"`
	Deleted    string `toml:"deleted"`
	HunkHeader string `toml:"hunk_header"`
	FileHeader string `toml:"file_header"`
}

func DefaultTheme() Theme {
	return Theme{
		Name: "Default",
		Chrome: ChromeColors{
			BarBg:         "236",
			TabActiveBg:   "63",
			TabActiveFg:   "255",
			TabInactiveBg: "235",
			ModalBg:       "235",
			ModalBorder:   "63",
			SelectionBg:   "237",
		},
		Text: TextColors{
			Bright: "255",
			Muted:  "245",
			Dim:    "241",
			Accent: "205",
			Zero:   "240",
		},
		Status: StatusColors{
			Positive: "82",
			Warning:  "208",
			Error:    "196",
			Info:     "63",
			Tag:      "214",
		},
		Charts: ChartColors{
			Gradient:        []string{"63", "33", "39", "49", "82", "154"},
			BarGradient:     []string{"22", "28", "34", "82", "154"},
			HeatmapLevels:   []string{"236", "22", "28", "34", "46"},
			HealthGradient:  []string{"22", "28", "172", "208", "196"},
			CadenceGradient: []string{"82", "154", "214", "208", "196"},
			MiniChart:       "82",
			ActiveLine:      "82",
			DormantLine:     "196",
		},
		Diff: DiffColors{
			Added:      "82",
			Deleted:    "196",
			HunkHeader: "39",
			FileHeader: "214",
		},
	}
}

func GrayscaleTheme() Theme {
	return Theme{
		Name: "Grayscale",
		Chrome: ChromeColors{
			BarBg:         "235",
			TabActiveBg:   "250",
			TabActiveFg:   "232",
			TabInactiveBg: "236",
			ModalBg:       "236",
			ModalBorder:   "250",
			SelectionBg:   "238",
		},
		Text: TextColors{
			Bright: "255",
			Muted:  "246",
			Dim:    "242",
			Accent: "252",
			Zero:   "240",
		},
		Status: StatusColors{
			Positive: "250",
			Warning:  "248",
			Error:    "244",
			Info:     "250",
			Tag:      "252",
		},
		Charts: ChartColors{
			Gradient:        []string{"240", "242", "244", "246", "248", "252"},
			BarGradient:     []string{"238", "240", "244", "248", "252"},
			HeatmapLevels:   []string{"236", "240", "244", "248", "252"},
			HealthGradient:  []string{"240", "244", "246", "248", "252"},
			CadenceGradient: []string{"240", "244", "246", "248", "252"},
			MiniChart:       "250",
			ActiveLine:      "250",
			DormantLine:     "242",
		},
		Diff: DiffColors{
			Added:      "250",
			Deleted:    "244",
			HunkHeader: "248",
			FileHeader: "252",
		},
	}
}

var builtinThemes = []Theme{DefaultTheme(), GrayscaleTheme()}

// Theme-derived global color variables, set by ApplyTheme.
var (
	chromeBg    lipgloss.Color
	tabActiveBg lipgloss.Color
	tabActiveFg lipgloss.Color
	tabInactiveBg lipgloss.Color
	modalBg     lipgloss.Color
	modalBorder lipgloss.Color
	selectionBg lipgloss.Color

	positiveColor lipgloss.Color
	warningColor  lipgloss.Color
	errorColor    lipgloss.Color
	infoColor     lipgloss.Color
	tagColor      lipgloss.Color
	zeroColor     lipgloss.Color

	barGradient     []lipgloss.Color
	heatmapLevels   []lipgloss.Color
	healthGradient  []lipgloss.Color
	cadenceGradient []lipgloss.Color
	miniChartColor  lipgloss.Color
	activeLineColor lipgloss.Color
	dormantLineColor lipgloss.Color

	diffAddedColor   lipgloss.Color
	diffDeletedColor lipgloss.Color
	diffHunkColor    lipgloss.Color
	diffFileColor    lipgloss.Color
)

// ApplyTheme sets all global color variables from the given theme.
func ApplyTheme(t Theme) {
	// Chrome
	chromeBg = lipgloss.Color(t.Chrome.BarBg)
	tabActiveBg = lipgloss.Color(t.Chrome.TabActiveBg)
	tabActiveFg = lipgloss.Color(t.Chrome.TabActiveFg)
	tabInactiveBg = lipgloss.Color(t.Chrome.TabInactiveBg)
	modalBg = lipgloss.Color(t.Chrome.ModalBg)
	modalBorder = lipgloss.Color(t.Chrome.ModalBorder)
	selectionBg = lipgloss.Color(t.Chrome.SelectionBg)

	// Text
	accentColor = lipgloss.Color(t.Text.Accent)
	dimColor = lipgloss.Color(t.Text.Dim)
	brightColor = lipgloss.Color(t.Text.Bright)
	mutedColor = lipgloss.Color(t.Text.Muted)
	zeroColor = lipgloss.Color(t.Text.Zero)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	dimStyle = lipgloss.NewStyle().Faint(true)
	boldStyle = lipgloss.NewStyle().Bold(true).Foreground(brightColor)
	mutedStyle = lipgloss.NewStyle().Foreground(mutedColor)

	// Status
	positiveColor = lipgloss.Color(t.Status.Positive)
	warningColor = lipgloss.Color(t.Status.Warning)
	errorColor = lipgloss.Color(t.Status.Error)
	infoColor = lipgloss.Color(t.Status.Info)
	tagColor = lipgloss.Color(t.Status.Tag)

	// Charts
	chartGradient = toColors(t.Charts.Gradient)
	barGradient = toColors(t.Charts.BarGradient)
	heatmapLevels = toColors(t.Charts.HeatmapLevels)
	healthGradient = toColors(t.Charts.HealthGradient)
	cadenceGradient = toColors(t.Charts.CadenceGradient)
	miniChartColor = lipgloss.Color(t.Charts.MiniChart)
	activeLineColor = lipgloss.Color(t.Charts.ActiveLine)
	dormantLineColor = lipgloss.Color(t.Charts.DormantLine)

	// Diff
	diffAddedColor = lipgloss.Color(t.Diff.Added)
	diffDeletedColor = lipgloss.Color(t.Diff.Deleted)
	diffHunkColor = lipgloss.Color(t.Diff.HunkHeader)
	diffFileColor = lipgloss.Color(t.Diff.FileHeader)
}

func toColors(ss []string) []lipgloss.Color {
	out := make([]lipgloss.Color, len(ss))
	for i, s := range ss {
		out[i] = lipgloss.Color(s)
	}
	return out
}

// ResolveTheme returns the theme matching the config's ThemeName,
// falling back to Default. If a CustomTheme is provided and ThemeName
// is "Custom", the custom theme is merged on top of Default.
func ResolveTheme(cfg Config) Theme {
	for _, t := range builtinThemes {
		if strings.EqualFold(t.Name, cfg.ThemeName) {
			return t
		}
	}
	if cfg.CustomTheme != nil {
		base := DefaultTheme()
		base.Name = "Custom"
		mergeTheme(&base, cfg.CustomTheme)
		return base
	}
	return DefaultTheme()
}

// availableThemeNames returns the list of theme names the user can cycle through.
func availableThemeNames(m model) []string {
	names := make([]string, len(builtinThemes))
	for i, t := range builtinThemes {
		names[i] = t.Name
	}
	if m.ToConfig().CustomTheme != nil {
		names = append(names, "Custom")
	}
	return names
}

// mergeTheme overlays non-zero values from src onto dst.
func mergeTheme(dst *Theme, src *Theme) {
	// Chrome
	mergeStr(&dst.Chrome.BarBg, src.Chrome.BarBg)
	mergeStr(&dst.Chrome.TabActiveBg, src.Chrome.TabActiveBg)
	mergeStr(&dst.Chrome.TabActiveFg, src.Chrome.TabActiveFg)
	mergeStr(&dst.Chrome.TabInactiveBg, src.Chrome.TabInactiveBg)
	mergeStr(&dst.Chrome.ModalBg, src.Chrome.ModalBg)
	mergeStr(&dst.Chrome.ModalBorder, src.Chrome.ModalBorder)
	mergeStr(&dst.Chrome.SelectionBg, src.Chrome.SelectionBg)

	// Text
	mergeStr(&dst.Text.Bright, src.Text.Bright)
	mergeStr(&dst.Text.Muted, src.Text.Muted)
	mergeStr(&dst.Text.Dim, src.Text.Dim)
	mergeStr(&dst.Text.Accent, src.Text.Accent)
	mergeStr(&dst.Text.Zero, src.Text.Zero)

	// Status
	mergeStr(&dst.Status.Positive, src.Status.Positive)
	mergeStr(&dst.Status.Warning, src.Status.Warning)
	mergeStr(&dst.Status.Error, src.Status.Error)
	mergeStr(&dst.Status.Info, src.Status.Info)
	mergeStr(&dst.Status.Tag, src.Status.Tag)

	// Charts
	mergeSlice(&dst.Charts.Gradient, src.Charts.Gradient)
	mergeSlice(&dst.Charts.BarGradient, src.Charts.BarGradient)
	mergeSlice(&dst.Charts.HeatmapLevels, src.Charts.HeatmapLevels)
	mergeSlice(&dst.Charts.HealthGradient, src.Charts.HealthGradient)
	mergeSlice(&dst.Charts.CadenceGradient, src.Charts.CadenceGradient)
	mergeStr(&dst.Charts.MiniChart, src.Charts.MiniChart)
	mergeStr(&dst.Charts.ActiveLine, src.Charts.ActiveLine)
	mergeStr(&dst.Charts.DormantLine, src.Charts.DormantLine)

	// Diff
	mergeStr(&dst.Diff.Added, src.Diff.Added)
	mergeStr(&dst.Diff.Deleted, src.Diff.Deleted)
	mergeStr(&dst.Diff.HunkHeader, src.Diff.HunkHeader)
	mergeStr(&dst.Diff.FileHeader, src.Diff.FileHeader)
}

func mergeStr(dst *string, src string) {
	if src != "" {
		*dst = src
	}
}

func mergeSlice(dst *[]string, src []string) {
	if len(src) > 0 {
		*dst = src
	}
}
