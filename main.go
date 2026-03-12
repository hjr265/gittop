package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-git/v5"
)

func main() {
	path := "."
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: not a git repository: %s\n", path)
		os.Exit(1)
	}

	m := newModel(repo, path)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
