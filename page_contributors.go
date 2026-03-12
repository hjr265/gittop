package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type contributorsPage struct{}

func newContributorsPage() *contributorsPage { return &contributorsPage{} }

func (p *contributorsPage) Init() tea.Cmd                        { return nil }
func (p *contributorsPage) Update(tea.Msg) (Page, tea.Cmd)       { return p, nil }
func (p *contributorsPage) View(width, height int) string {
	return placeholderView("Contributors", width, height)
}

func placeholderView(title string, width, height int) string {
	titleRendered := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("63")).
		Render(title)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Foreground(lipgloss.Color("245")).
		Width(30).
		Padding(1, 2).
		Render(fmt.Sprintf("%s\n\n%s",
			titleRendered,
			dimStyle.Render("Coming soon"),
		))

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
