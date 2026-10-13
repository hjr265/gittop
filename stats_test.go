package main

import (
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

// testRepo builds an in-memory repository one object at a time, so tests can
// create commits with explicit parents.
type testRepo struct {
	t     *testing.T
	repo  *git.Repository
	clock time.Time
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatalf("git.Init: %v", err)
	}
	return &testRepo{
		t:     t,
		repo:  repo,
		clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func (r *testRepo) store(o interface {
	Encode(plumbing.EncodedObject) error
}) plumbing.Hash {
	r.t.Helper()
	obj := r.repo.Storer.NewEncodedObject()
	if err := o.Encode(obj); err != nil {
		r.t.Fatalf("encode object: %v", err)
	}
	h, err := r.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		r.t.Fatalf("store object: %v", err)
	}
	return h
}

// tree stores a flat tree (no subdirectories) holding the given files.
func (r *testRepo) tree(files map[string]string) plumbing.Hash {
	r.t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	tree := &object.Tree{}
	for _, name := range names {
		obj := r.repo.Storer.NewEncodedObject()
		obj.SetType(plumbing.BlobObject)
		w, err := obj.Writer()
		if err != nil {
			r.t.Fatalf("blob writer: %v", err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			r.t.Fatalf("write blob: %v", err)
		}
		w.Close()
		h, err := r.repo.Storer.SetEncodedObject(obj)
		if err != nil {
			r.t.Fatalf("store blob: %v", err)
		}
		tree.Entries = append(tree.Entries, object.TreeEntry{Name: name, Mode: filemode.Regular, Hash: h})
	}
	return r.store(tree)
}

// commit stores a commit with the given files and parents. Each commit is
// dated one day after the previous one.
func (r *testRepo) commit(msg string, files map[string]string, parents ...plumbing.Hash) plumbing.Hash {
	r.t.Helper()
	sig := object.Signature{Name: "Test Author", Email: "test@example.com", When: r.clock}
	r.clock = r.clock.AddDate(0, 0, 1)
	return r.store(&object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      msg,
		TreeHash:     r.tree(files),
		ParentHashes: parents,
	})
}

func (r *testRepo) setRef(name plumbing.ReferenceName, h plumbing.Hash) {
	r.t.Helper()
	if err := r.repo.Storer.SetReference(plumbing.NewHashReference(name, h)); err != nil {
		r.t.Fatalf("set ref %s: %v", name, err)
	}
}

func (r *testRepo) branch(name string, h plumbing.Hash) {
	r.t.Helper()
	r.setRef(plumbing.NewBranchReferenceName(name), h)
}

func (r *testRepo) tag(name string, h plumbing.Hash) {
	r.t.Helper()
	r.setRef(plumbing.NewTagReferenceName(name), h)
}

// mergeFixture is a repository with a merged feature branch that has since
// moved on:
//
//	A --- B --- D --- M   (master, HEAD)
//	       \         /
//	        C ------+
//	         \
//	          E           (feature)
//
// v1 tags B. v2 tags E, which is not reachable from HEAD.
type mergeFixture struct {
	repo             *git.Repository
	a, b, c, d, m, e plumbing.Hash
}

func newMergeFixture(t *testing.T) *mergeFixture {
	t.Helper()
	r := newTestRepo(t)
	f := &mergeFixture{repo: r.repo}

	f.a = r.commit("A", map[string]string{"a.txt": "a\n"})
	f.b = r.commit("B", map[string]string{"a.txt": "a\n", "b.txt": "b\n"}, f.a)
	f.c = r.commit("C", map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "c\n"}, f.b)
	f.d = r.commit("D", map[string]string{"a.txt": "a\n", "b.txt": "b\n", "d.txt": "d\n"}, f.b)
	f.m = r.commit("M", map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "c\n", "d.txt": "d\n"}, f.d, f.c)
	f.e = r.commit("E", map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "c\n", "e.txt": "e\n"}, f.c)

	r.branch("master", f.m)
	r.branch("feature", f.e)
	r.tag("v1", f.b)
	r.tag("v2", f.e)
	return f
}

func TestCollectCommits(t *testing.T) {
	f := newMergeFixture(t)

	commits, err := CollectCommits(f.repo, false, LoadMailmap(f.repo))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}

	got := map[string]bool{}
	for _, c := range commits {
		got[c.Hash] = true
	}
	want := []plumbing.Hash{f.a, f.b, f.c, f.d, f.m}
	if len(commits) != len(want) {
		t.Errorf("CollectCommits returned %d commits, want %d", len(commits), len(want))
	}
	for _, h := range want {
		if !got[h.String()] {
			t.Errorf("CollectCommits is missing commit %s", h)
		}
	}
	if got[f.e.String()] {
		t.Errorf("CollectCommits returned %s, which is not reachable from HEAD", f.e)
	}
}

func TestCollectBranches(t *testing.T) {
	f := newMergeFixture(t)

	branches, err := CollectBranches(f.repo, LoadMailmap(f.repo))
	if err != nil {
		t.Fatalf("CollectBranches error: %v", err)
	}

	current := map[string]bool{}
	for _, b := range branches {
		current[b.Name] = b.IsCurrent
		if b.Name == "feature" && (b.Ahead != 1 || b.Behind != 2) {
			t.Errorf("branch %q is %d ahead, %d behind, want 1 ahead, 2 behind", b.Name, b.Ahead, b.Behind)
		}
	}
	want := map[string]bool{"master": true, "feature": false}
	if len(current) != len(want) {
		t.Errorf("CollectBranches returned %d branches, want %d", len(current), len(want))
	}
	for name, isCurrent := range want {
		got, ok := current[name]
		if !ok {
			t.Errorf("CollectBranches is missing branch %q", name)
			continue
		}
		if got != isCurrent {
			t.Errorf("branch %q IsCurrent = %v, want %v", name, got, isCurrent)
		}
	}
}

func TestCollectTags(t *testing.T) {
	f := newMergeFixture(t)

	tags, err := CollectTags(f.repo)
	if err != nil {
		t.Fatalf("CollectTags error: %v", err)
	}

	got := map[string]string{}
	for _, tag := range tags {
		got[tag.Name] = tag.CommitHash
	}
	want := map[string]string{"v1": f.b.String(), "v2": f.e.String()}
	if len(got) != len(want) {
		t.Errorf("CollectTags returned %d tags, want %d", len(got), len(want))
	}
	for name, hash := range want {
		if got[name] != hash {
			t.Errorf("tag %q points at %q, want %q", name, got[name], hash)
		}
	}
}

func TestCommitFiles(t *testing.T) {
	r := newTestRepo(t)
	root := r.commit("Add files", map[string]string{"a.txt": "a\n", "old.txt": "some content\n"})
	edit := r.commit("Edit a.txt", map[string]string{"a.txt": "a\na\n", "old.txt": "some content\n"}, root)
	rename := r.commit("Rename old.txt", map[string]string{"a.txt": "a\na\n", "new.txt": "some content\n"}, edit)

	tests := []struct {
		hash plumbing.Hash
		want []string
	}{
		// Root commit lists every file.
		{root, []string{"a.txt", "old.txt"}},
		{edit, []string{"a.txt"}},
		// A rename shows up as both the old and the new path.
		{rename, []string{"new.txt", "old.txt"}},
	}
	for _, tt := range tests {
		c, err := r.repo.CommitObject(tt.hash)
		if err != nil {
			t.Fatalf("CommitObject(%s) error: %v", tt.hash, err)
		}
		got := commitFiles(c)
		sort.Strings(got)
		if !slices.Equal(got, tt.want) {
			t.Errorf("commitFiles(%q) = %v, want %v", c.Message, got, tt.want)
		}
	}
}

func TestAheadBehind(t *testing.T) {
	f := newMergeFixture(t)
	base := loadCommitGraph(f.repo, f.m)

	tests := []struct {
		name                  string
		hash                  plumbing.Hash
		wantAhead, wantBehind int
	}{
		{"M", f.m, 0, 0},
		// E is ahead by itself and lacks D and M.
		{"E", f.e, 1, 2},
		// C was merged, so only D and M are missing.
		{"C", f.c, 0, 2},
		{"B", f.b, 0, 3},
		{"A", f.a, 0, 4},
	}
	for _, tt := range tests {
		ahead, behind := aheadBehind(f.repo, base, tt.hash)
		if ahead != tt.wantAhead || behind != tt.wantBehind {
			t.Errorf("aheadBehind(%s) = %d ahead, %d behind, want %d ahead, %d behind",
				tt.name, ahead, behind, tt.wantAhead, tt.wantBehind)
		}
	}
}

func TestCommitsSinceLatestTag(t *testing.T) {
	f := newMergeFixture(t)

	tags, err := CollectTags(f.repo)
	if err != nil {
		t.Fatalf("CollectTags error: %v", err)
	}

	// v2 is newer but not reachable from HEAD, so v1 is used. HEAD has C, D
	// and M on top of it.
	name, count := CommitsSinceLatestTag(f.repo, tags)
	if name != "v1" || count != 3 {
		t.Errorf("CommitsSinceLatestTag = %q, %d, want %q, %d", name, count, "v1", 3)
	}

	// No reachable tag at all.
	name, count = CommitsSinceLatestTag(f.repo, tags[:1])
	if name != "" || count != 0 {
		t.Errorf("CommitsSinceLatestTag with only v2 = %q, %d, want %q, %d", name, count, "", 0)
	}
}

func TestMergeCommitFiles(t *testing.T) {
	f := newMergeFixture(t)

	commits, err := CollectCommits(f.repo, true, LoadMailmap(f.repo))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}
	for _, c := range commits {
		if c.Hash == f.m.String() && len(c.Files) != 0 {
			t.Errorf("merge commit lists files %v, want none", c.Files)
		}
	}

	// c.txt was added in C and reached HEAD through the merge M. Only C
	// counts as a change to it.
	for _, h := range BuildHealthData(map[string]int{"c.txt": 1}, commits, false) {
		if h.Path == "c.txt" && h.Churn != 1 {
			t.Errorf("c.txt churn = %d, want 1", h.Churn)
		}
	}
}
