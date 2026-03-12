package main

import tea "github.com/charmbracelet/bubbletea"

type filesPage struct{}

func newFilesPage() *filesPage                          { return &filesPage{} }
func (p *filesPage) Init() tea.Cmd                      { return nil }
func (p *filesPage) Update(tea.Msg) (Page, tea.Cmd)     { return p, nil }
func (p *filesPage) View(width, height int) string {
	return placeholderView("Files", width, height)
}
