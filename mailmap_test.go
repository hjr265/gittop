package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

func TestMailmapResolve(t *testing.T) {
	content := []byte(`# comment
Proper Name <proper@example.com>
<canonical@example.com> <old@example.com>
Full Name <full@example.com> <alt@example.com>
Real Name <real@example.com> Typo Name <typo@example.com>
`)

	m := &Mailmap{
		byEmail:     make(map[string]mailmapMailbox),
		byNameEmail: make(map[mailmapMailbox]mailmapMailbox),
	}
	m.parse(content)

	tests := []struct {
		inName, inEmail     string
		wantName, wantEmail string
	}{
		// Format 1: "Proper Name <proper@example.com>" — replaces name for matching email.
		{"Old Name", "proper@example.com", "Proper Name", "proper@example.com"},
		// Format 2: "<canonical@example.com> <old@example.com>" — replaces email.
		{"Someone", "old@example.com", "Someone", "canonical@example.com"},
		// Format 3: "Full Name <full@example.com> <alt@example.com>" — replaces both.
		{"Whatever", "alt@example.com", "Full Name", "full@example.com"},
		// Format 4: "Real Name <real@example.com> Typo Name <typo@example.com>" — specific match.
		{"Typo Name", "typo@example.com", "Real Name", "real@example.com"},
		// Format 4 should not match different name with same email.
		{"Other Name", "typo@example.com", "Other Name", "typo@example.com"},
		// No match — pass through.
		{"Unknown", "unknown@example.com", "Unknown", "unknown@example.com"},
		// Case-insensitive email match.
		{"Old Name", "PROPER@EXAMPLE.COM", "Proper Name", "proper@example.com"},
	}

	for _, tt := range tests {
		gotName, gotEmail := m.Resolve(tt.inName, tt.inEmail)
		if gotName != tt.wantName || gotEmail != tt.wantEmail {
			t.Errorf("Resolve(%q, %q) = (%q, %q), want (%q, %q)",
				tt.inName, tt.inEmail, gotName, gotEmail, tt.wantName, tt.wantEmail)
		}
	}
}

func TestMailmapNil(t *testing.T) {
	var m *Mailmap
	name, email := m.Resolve("Test", "test@example.com")
	if name != "Test" || email != "test@example.com" {
		t.Errorf("nil Mailmap should pass through, got (%q, %q)", name, email)
	}
}

// isolateGitConfig points the user-level Git config at an empty directory
// and returns it, so tests do not pick up the developer's own settings.
func isolateGitConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newMailmapRepo creates an on-disk repository whose HEAD commit holds the
// given .mailmap.
func newMailmapRepo(t *testing.T, committed string) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit error: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree error: %v", err)
	}
	writeFile(t, filepath.Join(dir, ".mailmap"), committed)
	if _, err := wt.Add(".mailmap"); err != nil {
		t.Fatalf("Add error: %v", err)
	}
	_, err = wt.Commit("Add mailmap", &git.CommitOptions{
		Author: &object.Signature{Name: "Test Author", Email: "test@example.com"},
	})
	if err != nil {
		t.Fatalf("Commit error: %v", err)
	}
	return repo, dir
}

func checkResolve(t *testing.T, m *Mailmap, email, wantName string) {
	t.Helper()
	if name, _ := m.Resolve("Someone", email); name != wantName {
		t.Errorf("Resolve(%q) name = %q, want %q", email, name, wantName)
	}
}

func TestLoadMailmapWorkingTree(t *testing.T) {
	isolateGitConfig(t)
	repo, dir := newMailmapRepo(t, "Committed <a@example.com>\n")

	// An uncommitted edit is what Git uses.
	writeFile(t, filepath.Join(dir, ".mailmap"), "Working <a@example.com>\n")
	checkResolve(t, LoadMailmap(repo), "a@example.com", "Working")

	// With the file gone, the committed copy is not consulted.
	if err := os.Remove(filepath.Join(dir, ".mailmap")); err != nil {
		t.Fatal(err)
	}
	checkResolve(t, LoadMailmap(repo), "a@example.com", "Someone")
}

func TestLoadMailmapBare(t *testing.T) {
	isolateGitConfig(t)
	repo, err := git.Init(memory.NewStorage(), nil)
	if err != nil {
		t.Fatalf("git.Init error: %v", err)
	}
	r := &testRepo{t: t, repo: repo}
	r.branch("master", r.commit("A", map[string]string{".mailmap": "Committed <a@example.com>\n"}))

	checkResolve(t, LoadMailmap(repo), "a@example.com", "Committed")
}

func TestLoadMailmapConfig(t *testing.T) {
	home := isolateGitConfig(t)
	repo, dir := newMailmapRepo(t, "Blob <a@example.com>\nBlob <b@example.com>\n")
	writeFile(t, filepath.Join(dir, ".mailmap"), "Working <c@example.com>\n")

	// mailmap.file in the user's config, with a tilde path.
	writeFile(t, filepath.Join(home, ".gitconfig"), "[mailmap]\n\tfile = ~/mailmap\n")
	writeFile(t, filepath.Join(home, "mailmap"), "File <a@example.com>\n")

	// mailmap.blob in the repository's config.
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("Config error: %v", err)
	}
	cfg.Raw.Section("mailmap").SetOption("blob", "HEAD:.mailmap")
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig error: %v", err)
	}

	m := LoadMailmap(repo)
	// Both name a@example.com; the file is applied after the blob.
	checkResolve(t, m, "a@example.com", "File")
	checkResolve(t, m, "b@example.com", "Blob")
	checkResolve(t, m, "c@example.com", "Working")
}
