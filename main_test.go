package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	// The exclusive handle must still see the linked worktree's HEAD.
	exclusive, done := openExclusive(opened)
	defer done()
	if head, err := exclusive.Head(); err != nil || head.Name() != feature {
		t.Errorf("exclusive HEAD = %v (err %v), want %s", head, err, feature)
	}
}

func TestOpenExclusive(t *testing.T) {
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit error: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree error: %v", err)
	}
	for i, msg := range []string{"A", "B", "C"} {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(msg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add("a.txt"); err != nil {
			t.Fatalf("Add error: %v", err)
		}
		if _, err := wt.Commit(msg, &git.CommitOptions{
			Author: &object.Signature{Name: "Test Author", Email: "test@example.com", When: time.Unix(int64(i)*86400, 0)},
		}); err != nil {
			t.Fatalf("Commit error: %v", err)
		}
	}
	// Move the objects into a pack, which is where kept descriptors apply.
	if err := repo.RepackObjects(&git.RepackConfig{}); err != nil {
		t.Fatalf("RepackObjects error: %v", err)
	}

	opened, err := openRepository(dir)
	if err != nil {
		t.Fatalf("openRepository error: %v", err)
	}
	exclusive, done := openExclusive(opened)
	defer done()
	if exclusive == opened {
		t.Fatal("openExclusive returned the shared handle")
	}
	commits, err := CollectCommits(exclusive, LoadMailmap(exclusive))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}
	if len(commits) != 3 {
		t.Errorf("CollectCommits returned %d commits, want 3", len(commits))
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    cliOptions
		wantErr error // nil, flag.ErrHelp, or errAny
	}{
		{"no args", nil, cliOptions{}, nil},
		{"path", []string{"/repo"}, cliOptions{path: "/repo"}, nil},
		{"version", []string{"--version"}, cliOptions{version: true}, nil},
		{"single dash", []string{"-version"}, cliOptions{version: true}, nil},
		{"help", []string{"--help"}, cliOptions{}, flag.ErrHelp},
		{"unknown flag", []string{"--nope"}, cliOptions{}, errAny},
		{"too many args", []string{"a", "b"}, cliOptions{path: "a"}, errAny},
		{"flag after path", []string{"/repo", "--version"}, cliOptions{path: "/repo"}, errAny},
		{"range", []string{"--range", "2y", "/repo"}, cliOptions{path: "/repo", rangeLabel: "2y"}, nil},
		{"range any case", []string{"--range=ALL"}, cliOptions{rangeLabel: "ALL"}, nil},
		{"invalid range", []string{"--range", "7y"}, cliOptions{}, errAny},
	}
	for _, tt := range tests {
		var out strings.Builder
		got, err := parseArgs(tt.args, &out)
		switch {
		case tt.wantErr == nil && err != nil:
			t.Errorf("%s: unexpected error %v", tt.name, err)
		case tt.wantErr == errAny && err == nil:
			t.Errorf("%s: expected an error", tt.name)
		case tt.wantErr == flag.ErrHelp && !errors.Is(err, flag.ErrHelp):
			t.Errorf("%s: error = %v, want flag.ErrHelp", tt.name, err)
		}
		if err == nil && got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
		if err != nil && !strings.Contains(out.String(), "Usage: gittop") {
			t.Errorf("%s: usage not printed on error, got %q", tt.name, out.String())
		}
	}
}

// errAny marks a test case that expects some error.
var errAny = errors.New("any error")
