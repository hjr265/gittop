package main

import tea "github.com/charmbracelet/bubbletea"

type logPage struct{}

func newLogPage() *logPage                          { return &logPage{} }
func (p *logPage) Init() tea.Cmd                    { return nil }
func (p *logPage) Update(tea.Msg) (Page, tea.Cmd)   { return p, nil }
func (p *logPage) View(width, height int) string {
	return placeholderView("Log", width, height)
}
