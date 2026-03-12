package main

import tea "github.com/charmbracelet/bubbletea"

type activityPage struct{}

func newActivityPage() *activityPage { return &activityPage{} }

func (p *activityPage) Init() tea.Cmd                   { return nil }
func (p *activityPage) Update(tea.Msg) (Page, tea.Cmd)  { return p, nil }
func (p *activityPage) View(width, height int) string {
	return placeholderView("Activity", width, height)
}
