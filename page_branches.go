package main

import tea "github.com/charmbracelet/bubbletea"

type branchesPage struct{}

func newBranchesPage() *branchesPage                          { return &branchesPage{} }
func (p *branchesPage) Init() tea.Cmd                         { return nil }
func (p *branchesPage) Update(tea.Msg) (Page, tea.Cmd)        { return p, nil }
func (p *branchesPage) View(width, height int) string {
	return placeholderView("Branches", width, height)
}
