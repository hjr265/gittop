package main

import (
	"maps"
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

	commits, err := CollectCommits(f.repo, LoadMailmap(f.repo))
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

	commits, err := CollectCommits(f.repo, LoadMailmap(f.repo))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}
	commits, err = WithCommitFiles(f.repo, commits)
	if err != nil {
		t.Fatalf("WithCommitFiles error: %v", err)
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

func TestCollectCommitsOrder(t *testing.T) {
	f := newMergeFixture(t)

	commits, err := CollectCommits(f.repo, LoadMailmap(f.repo))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}

	// Newest first. The default depth-first order would put C last.
	want := []plumbing.Hash{f.m, f.d, f.c, f.b, f.a}
	var got []string
	for _, c := range commits {
		got = append(got, c.Message)
	}
	if len(commits) != len(want) {
		t.Fatalf("CollectCommits returned %v, want %d commits", got, len(want))
	}
	for i, h := range want {
		if commits[i].Hash != h.String() {
			t.Errorf("CollectCommits order = %v, want [M D C B A]", got)
			break
		}
	}
}

func TestCollectCommitsMissingParent(t *testing.T) {
	// As in a shallow clone: B's parent is not in the repository, and B is
	// reached through one side of a merge.
	r := newTestRepo(t)
	missing := plumbing.NewHash("1111111111111111111111111111111111111111")
	b := r.commit("B", map[string]string{"a.txt": "a\n"}, missing)
	c := r.commit("C", map[string]string{"a.txt": "a\n", "c.txt": "c\n"}, b)
	d := r.commit("D", map[string]string{"a.txt": "a\n", "d.txt": "d\n"}, b)
	m := r.commit("M", map[string]string{"a.txt": "a\n", "c.txt": "c\n", "d.txt": "d\n"}, d, c)
	r.branch("master", m)

	commits, err := CollectCommits(r.repo, LoadMailmap(r.repo))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}
	if len(commits) != 4 {
		t.Errorf("CollectCommits returned %d commits, want 4", len(commits))
	}
}

func TestWithCommitFiles(t *testing.T) {
	f := newMergeFixture(t)

	commits, err := CollectCommits(f.repo, LoadMailmap(f.repo))
	if err != nil {
		t.Fatalf("CollectCommits error: %v", err)
	}
	withFiles, err := WithCommitFiles(f.repo, commits)
	if err != nil {
		t.Fatalf("WithCommitFiles error: %v", err)
	}

	want := map[string][]string{
		"A": {"a.txt"},
		"B": {"b.txt"},
		"C": {"c.txt"},
		"D": {"d.txt"},
		"M": nil,
	}
	if len(withFiles) != len(commits) {
		t.Fatalf("WithCommitFiles returned %d commits, want %d", len(withFiles), len(commits))
	}
	for i, c := range withFiles {
		if c.Hash != commits[i].Hash {
			t.Errorf("commit %d is %s, want %s", i, c.Hash, commits[i].Hash)
		}
		if !slices.Equal(c.Files, want[c.Message]) {
			t.Errorf("files of %s = %v, want %v", c.Message, c.Files, want[c.Message])
		}
		if commits[i].Files != nil {
			t.Errorf("WithCommitFiles modified its input: %s has files %v", commits[i].Message, commits[i].Files)
		}
	}
}

func TestCollectFileLineCounts(t *testing.T) {
	r := newTestRepo(t)
	r.branch("master", r.commit("Add files", map[string]string{
		"text.txt":       "one\ntwo\nthree\n",
		"no-newline.txt": "one\ntwo",
		"empty.txt":      "",
		"image.png":      "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
	}))

	got, err := CollectFileLineCounts(r.repo)
	if err != nil {
		t.Fatalf("CollectFileLineCounts error: %v", err)
	}
	want := map[string]int{"text.txt": 3, "no-newline.txt": 2, "empty.txt": 0}
	if !maps.Equal(got, want) {
		t.Errorf("CollectFileLineCounts = %v, want %v", got, want)
	}
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestTruncateToDay(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  time.Time
	}{
		{
			name:  "strips hours and minutes",
			input: time.Date(2025, 6, 15, 14, 30, 45, 123, time.UTC),
			want:  date(2025, 6, 15),
		},
		{
			name:  "midnight stays midnight",
			input: date(2025, 1, 1),
			want:  date(2025, 1, 1),
		},
		{
			name:  "non-UTC timezone normalizes to UTC",
			input: time.Date(2025, 3, 10, 23, 59, 59, 0, time.FixedZone("EST", -5*3600)),
			want:  date(2025, 3, 10),
		},
	}
	for _, tt := range tests {
		got := truncateToDay(tt.input)
		if !got.Equal(tt.want) {
			t.Errorf("truncateToDay(%v) [%s] = %v, want %v", tt.input, tt.name, got, tt.want)
		}
	}
}

func TestBucketKey(t *testing.T) {
	tests := []struct {
		name string
		date time.Time
		gran Granularity
		want time.Time
	}{
		// Daily returns the input unchanged.
		{
			name: "daily passthrough",
			date: date(2025, 6, 15),
			gran: GranularityDaily,
			want: date(2025, 6, 15),
		},
		// Weekly: Monday maps to itself.
		{
			name: "weekly monday",
			date: date(2025, 6, 16), // Monday
			gran: GranularityWeekly,
			want: date(2025, 6, 16),
		},
		// Weekly: Wednesday maps to preceding Monday.
		{
			name: "weekly wednesday",
			date: date(2025, 6, 18), // Wednesday
			gran: GranularityWeekly,
			want: date(2025, 6, 16),
		},
		// Weekly: Sunday maps to preceding Monday.
		{
			name: "weekly sunday",
			date: date(2025, 6, 22), // Sunday
			gran: GranularityWeekly,
			want: date(2025, 6, 16),
		},
		// Monthly: any day maps to the 1st.
		{
			name: "monthly",
			date: date(2025, 6, 18),
			gran: GranularityMonthly,
			want: date(2025, 6, 1),
		},
		// Yearly: any day maps to Jan 1.
		{
			name: "yearly",
			date: date(2025, 6, 18),
			gran: GranularityYearly,
			want: date(2025, 1, 1),
		},
	}
	for _, tt := range tests {
		got := bucketKey(tt.date, tt.gran)
		if !got.Equal(tt.want) {
			t.Errorf("bucketKey(%v, %v) [%s] = %v, want %v", tt.date, tt.gran, tt.name, got, tt.want)
		}
	}
}

func TestGranularityString(t *testing.T) {
	tests := []struct {
		g    Granularity
		want string
	}{
		{GranularityDaily, "daily"},
		{GranularityWeekly, "weekly"},
		{GranularityMonthly, "monthly"},
		{GranularityYearly, "yearly"},
	}
	for _, tt := range tests {
		if got := tt.g.String(); got != tt.want {
			t.Errorf("Granularity(%d).String() = %q, want %q", tt.g, got, tt.want)
		}
	}
}

func TestGranularityNext(t *testing.T) {
	tests := []struct {
		g    Granularity
		want Granularity
	}{
		{GranularityDaily, GranularityWeekly},
		{GranularityWeekly, GranularityMonthly},
		{GranularityMonthly, GranularityYearly},
		{GranularityYearly, GranularityDaily},
	}
	for _, tt := range tests {
		if got := tt.g.Next(); got != tt.want {
			t.Errorf("Granularity(%d).Next() = %d, want %d", tt.g, got, tt.want)
		}
	}
}

func TestCommitsToDailyStats(t *testing.T) {
	t.Run("nil on empty input", func(t *testing.T) {
		got := CommitsToDailyStats(nil)
		if got != nil {
			t.Errorf("CommitsToDailyStats(nil) = %v, want nil", got)
		}
	})

	t.Run("single commit", func(t *testing.T) {
		commits := []CommitInfo{
			{Date: date(2025, 6, 15)},
		}
		got := CommitsToDailyStats(commits)
		if len(got) == 0 {
			t.Fatal("expected at least one stat")
		}
		if !got[0].Date.Equal(date(2025, 6, 15)) {
			t.Errorf("first stat date = %v, want 2025-06-15", got[0].Date)
		}
		if got[0].Count != 1 {
			t.Errorf("first stat count = %d, want 1", got[0].Count)
		}
	})

	t.Run("multiple commits same day", func(t *testing.T) {
		d := date(2025, 6, 15)
		commits := []CommitInfo{
			{Date: d},
			{Date: d},
			{Date: d},
		}
		got := CommitsToDailyStats(commits)
		if len(got) == 0 {
			t.Fatal("expected at least one stat")
		}
		if got[0].Count != 3 {
			t.Errorf("count = %d, want 3", got[0].Count)
		}
	})

	t.Run("fills gaps with zero-count days", func(t *testing.T) {
		commits := []CommitInfo{
			{Date: date(2025, 6, 15)},
			{Date: date(2025, 6, 18)},
		}
		got := CommitsToDailyStats(commits)
		if len(got) < 4 {
			t.Fatalf("expected at least 4 stats, got %d", len(got))
		}
		byDate := map[time.Time]int{}
		for _, s := range got {
			byDate[s.Date] = s.Count
		}
		for _, d := range []time.Time{date(2025, 6, 15), date(2025, 6, 16), date(2025, 6, 17), date(2025, 6, 18)} {
			if _, ok := byDate[d]; !ok {
				t.Errorf("expected date %v in output, not found", d)
			}
		}
		if byDate[date(2025, 6, 15)] != 1 {
			t.Errorf("2025-06-15 count = %d, want 1", byDate[date(2025, 6, 15)])
		}
		if byDate[date(2025, 6, 16)] != 0 {
			t.Errorf("2025-06-16 count = %d, want 0", byDate[date(2025, 6, 16)])
		}
		if byDate[date(2025, 6, 17)] != 0 {
			t.Errorf("2025-06-17 count = %d, want 0", byDate[date(2025, 6, 17)])
		}
		if byDate[date(2025, 6, 18)] != 1 {
			t.Errorf("2025-06-18 count = %d, want 1", byDate[date(2025, 6, 18)])
		}
	})

	t.Run("starts at earliest commit", func(t *testing.T) {
		commits := []CommitInfo{
			{Date: date(2025, 6, 20)},
			{Date: date(2025, 6, 15)},
		}
		got := CommitsToDailyStats(commits)
		if !got[0].Date.Equal(date(2025, 6, 15)) {
			t.Errorf("first date = %v, want 2025-06-15", got[0].Date)
		}
	})
}

func TestAggregateStats(t *testing.T) {
	t.Run("daily returns input unchanged", func(t *testing.T) {
		daily := []DayStat{
			{Date: date(2025, 6, 15), Count: 3},
			{Date: date(2025, 6, 16), Count: 1},
		}
		got := AggregateStats(daily, GranularityDaily)
		if len(got) != len(daily) {
			t.Fatalf("len = %d, want %d", len(got), len(daily))
		}
		for i := range got {
			if got[i] != daily[i] {
				t.Errorf("got[%d] = %v, want %v", i, got[i], daily[i])
			}
		}
	})

	t.Run("empty returns empty", func(t *testing.T) {
		got := AggregateStats(nil, GranularityWeekly)
		if got != nil {
			t.Errorf("AggregateStats(nil) = %v, want nil", got)
		}
	})

	t.Run("weekly groups by monday", func(t *testing.T) {
		daily := []DayStat{
			{Date: date(2025, 6, 16), Count: 2}, // Monday
			{Date: date(2025, 6, 17), Count: 3}, // Tuesday, same week
			{Date: date(2025, 6, 23), Count: 1}, // Next Monday
		}
		got := AggregateStats(daily, GranularityWeekly)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].Count != 5 {
			t.Errorf("first week count = %d, want 5", got[0].Count)
		}
		if got[1].Count != 1 {
			t.Errorf("second week count = %d, want 1", got[1].Count)
		}
	})

	t.Run("monthly groups by first of month", func(t *testing.T) {
		daily := []DayStat{
			{Date: date(2025, 6, 5), Count: 1},
			{Date: date(2025, 6, 20), Count: 4},
			{Date: date(2025, 7, 1), Count: 2},
		}
		got := AggregateStats(daily, GranularityMonthly)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].Count != 5 {
			t.Errorf("june count = %d, want 5", got[0].Count)
		}
		if got[1].Count != 2 {
			t.Errorf("july count = %d, want 2", got[1].Count)
		}
	})

	t.Run("yearly groups by jan 1", func(t *testing.T) {
		daily := []DayStat{
			{Date: date(2024, 3, 10), Count: 1},
			{Date: date(2024, 11, 5), Count: 2},
			{Date: date(2025, 1, 15), Count: 7},
		}
		got := AggregateStats(daily, GranularityYearly)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].Count != 3 {
			t.Errorf("2024 count = %d, want 3", got[0].Count)
		}
		if got[1].Count != 7 {
			t.Errorf("2025 count = %d, want 7", got[1].Count)
		}
	})

	t.Run("results sorted chronologically", func(t *testing.T) {
		daily := []DayStat{
			{Date: date(2025, 8, 1), Count: 1},
			{Date: date(2025, 6, 1), Count: 2},
			{Date: date(2025, 7, 1), Count: 3},
		}
		got := AggregateStats(daily, GranularityMonthly)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		for i := 1; i < len(got); i++ {
			if !got[i-1].Date.Before(got[i].Date) {
				t.Errorf("not sorted: got[%d].Date=%v >= got[%d].Date=%v", i-1, got[i-1].Date, i, got[i].Date)
			}
		}
	})
}

func TestFilterCommits(t *testing.T) {
	commits := []CommitInfo{
		{Author: "alice", Message: "add feature"},
		{Author: "bob", Message: "fix bug"},
		{Author: "alice", Message: "update docs"},
	}

	t.Run("nil expr returns all", func(t *testing.T) {
		got := FilterCommits(commits, nil)
		if len(got) != 3 {
			t.Errorf("len = %d, want 3", len(got))
		}
	})

	t.Run("filters by author", func(t *testing.T) {
		expr, err := ParseFilter(`author:alice`)
		if err != nil {
			t.Fatal(err)
		}
		got := FilterCommits(commits, expr)
		if len(got) != 2 {
			t.Errorf("len = %d, want 2", len(got))
		}
		for _, c := range got {
			if c.Author != "alice" {
				t.Errorf("unexpected author %q", c.Author)
			}
		}
	})

	t.Run("filters by message", func(t *testing.T) {
		expr, err := ParseFilter(`"fix"`)
		if err != nil {
			t.Fatal(err)
		}
		got := FilterCommits(commits, expr)
		if len(got) != 1 {
			t.Errorf("len = %d, want 1", len(got))
		}
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		expr, err := ParseFilter(`author:nobody`)
		if err != nil {
			t.Fatal(err)
		}
		got := FilterCommits(commits, expr)
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}

func TestBuildHealthData(t *testing.T) {
	lineCounts := map[string]int{
		"main.go":   100,
		"utils.go":  50,
		"README.md": 30,
	}
	commits := []CommitInfo{
		{Author: "alice", Date: date(2025, 6, 15), Files: []string{"main.go"}},
		{Author: "bob", Date: date(2025, 6, 16), Files: []string{"main.go", "utils.go"}},
		{Author: "alice", Date: date(2025, 6, 20), Files: []string{"main.go"}},
	}

	t.Run("unfiltered includes all files from lineCounts", func(t *testing.T) {
		got := BuildHealthData(lineCounts, commits, false)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		byPath := map[string]FileHealthInfo{}
		for _, f := range got {
			byPath[f.Path] = f
		}

		main := byPath["main.go"]
		if main.Lines != 100 {
			t.Errorf("main.go lines = %d, want 100", main.Lines)
		}
		if main.AuthorCount != 2 {
			t.Errorf("main.go authors = %d, want 2", main.AuthorCount)
		}
		if main.Churn != 3 {
			t.Errorf("main.go churn = %d, want 3", main.Churn)
		}
		if !main.LastChanged.Equal(date(2025, 6, 20)) {
			t.Errorf("main.go last changed = %v, want 2025-06-20", main.LastChanged)
		}

		readme := byPath["README.md"]
		if readme.Lines != 30 {
			t.Errorf("README.md lines = %d, want 30", readme.Lines)
		}
		if readme.Churn != 0 {
			t.Errorf("README.md churn = %d, want 0 (not in commits)", readme.Churn)
		}
	})

	t.Run("filtered only includes files from commits", func(t *testing.T) {
		got := BuildHealthData(lineCounts, commits, true)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2 (main.go, utils.go)", len(got))
		}
		for _, f := range got {
			if f.Path == "README.md" {
				t.Error("filtered mode should not include README.md (not in commits)")
			}
		}
	})

	t.Run("empty commits", func(t *testing.T) {
		got := BuildHealthData(lineCounts, nil, false)
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		for _, f := range got {
			if f.Churn != 0 || f.AuthorCount != 0 {
				t.Errorf("%s: expected zero churn/authors with no commits, got churn=%d authors=%d",
					f.Path, f.Churn, f.AuthorCount)
			}
		}
	})

	t.Run("filtered with empty commits returns nothing", func(t *testing.T) {
		got := BuildHealthData(lineCounts, nil, true)
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}
