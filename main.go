package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

func openBareRepository(path string) (*git.Repository, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fs := osfs.New(path)
	stor := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	return git.Open(stor, nil)
}

func main() {
	path := "."
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		repo, err = openBareRepository(path)
	}
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
