package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestOpenRepositoryLinkedWorktree(t *testing.T) {
	dir := t.TempDir()
	mainDir := filepath.Join(dir, "main")
	wtDir := filepath.Join(dir, "wt")

	// A repository with one commit on its default branch.
	repo, err := git.PlainInit(mainDir, false)
	if err != nil {
		t.Fatalf("PlainInit error: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("a.txt"); err != nil {
		t.Fatalf("Add error: %v", err)
	}
	hash, err := wt.Commit("A", &git.CommitOptions{
		Author: &object.Signature{Name: "Test Author", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit error: %v", err)
	}

	// Lay out a linked worktree on branch "feature" the way
	// `git worktree add` does.
	feature := plumbing.NewBranchReferenceName("feature")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(feature, hash)); err != nil {
		t.Fatalf("SetReference error: %v", err)
	}
	adminDir := filepath.Join(mainDir, ".git", "worktrees", "wt")
	files := map[string]string{
		filepath.Join(adminDir, "HEAD"):      "ref: refs/heads/feature\n",
		filepath.Join(adminDir, "commondir"): "../..\n",
		filepath.Join(adminDir, "gitdir"):    filepath.Join(wtDir, ".git") + "\n",
		filepath.Join(wtDir, ".git"):         "gitdir: " + adminDir + "\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	opened, err := openRepository(wtDir)
	if err != nil {
		t.Fatalf("openRepository error: %v", err)
	}
	head, err := opened.Head()
	if err != nil {
		t.Fatalf("Head error: %v", err)
	}
	if head.Name() != feature {
		t.Errorf("HEAD is %s, want %s", head.Name(), feature)
	}
	commits, err := CollectCommits(opened, LoadMailmap(opened))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}
	if len(commits) != 1 {
		t.Errorf("CollectCommits returned %d commits, want 1", len(commits))
	}
}
