package main

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DayStat holds the commit count for a single calendar day.
type DayStat struct {
	Date  time.Time
	Count int
}

// CommitInfo holds metadata for a single commit.
type CommitInfo struct {
	Hash    string
	Author  string
	Email   string
	Date    time.Time
	Hour    int
	Weekday time.Weekday
	Month   time.Month
	Message string
	Files   []string // populated only when needed (path filter)
}

// CollectCommits walks the entire commit log reachable from HEAD and
// returns per-commit metadata. If needFiles is true, each commit's
// changed file paths are computed (slower).
func CollectCommits(repo *git.Repository, needFiles bool, mm *Mailmap) ([]CommitInfo, error) {
	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}

	var commits []CommitInfo
	err = iter.ForEach(func(c *object.Commit) error {
		when := c.Author.When
		name, email := mm.Resolve(c.Author.Name, c.Author.Email)
		ci := CommitInfo{
			Hash:    c.Hash.String(),
			Author:  name,
			Email:   email,
			Date:    truncateToDay(when),
			Hour:    when.Hour(),
			Weekday: when.Weekday(),
			Month:   when.Month(),
			Message: strings.TrimSpace(c.Message),
		}
		if needFiles {
			ci.Files = commitFiles(c)
		}
		commits = append(commits, ci)
		return nil
	})
	if err != nil && err != plumbing.ErrObjectNotFound {
		return nil, err
	}

	return commits, nil
}

// commitFiles returns the list of file paths changed by a commit.
func commitFiles(c *object.Commit) []string {
	tree, err := c.Tree()
	if err != nil {
		return nil
	}

	var parentTree *object.Tree
	if c.NumParents() > 0 {
		parent, err := c.Parents().Next()
		if err == nil {
			parentTree, _ = parent.Tree()
		}
	}

	if parentTree == nil {
		// Root commit — list all files.
		var files []string
		tree.Files().ForEach(func(f *object.File) error {
			files = append(files, f.Name)
			return nil
		})
		return files
	}

	changes, err := parentTree.Diff(tree)
	if err != nil {
		return nil
	}

	var files []string
	for _, ch := range changes {
		if ch.From.Name != "" {
			files = append(files, ch.From.Name)
		}
		if ch.To.Name != "" && ch.To.Name != ch.From.Name {
			files = append(files, ch.To.Name)
		}
	}
	return files
}

// CommitsToDailyStats aggregates commits into daily counts.
func CommitsToDailyStats(commits []CommitInfo) []DayStat {
	if len(commits) == 0 {
		return nil
	}

	counts := make(map[time.Time]int)
	var earliest time.Time
	for _, c := range commits {
		counts[c.Date]++
		if earliest.IsZero() || c.Date.Before(earliest) {
			earliest = c.Date
		}
	}

	now := truncateToDay(time.Now())
	var stats []DayStat
	for d := earliest; !d.After(now); d = d.AddDate(0, 0, 1) {
		stats = append(stats, DayStat{Date: d, Count: counts[d]})
	}
	return stats
}

// FilterCommits returns only commits matching the given filter expression.
func FilterCommits(commits []CommitInfo, expr FilterExpr) []CommitInfo {
	if expr == nil {
		return commits
	}
	var result []CommitInfo
	for i := range commits {
		if expr.Match(&commits[i]) {
			result = append(result, commits[i])
		}
	}
	return result
}

// Granularity represents the time bucket size for chart aggregation.
type Granularity int

const (
	GranularityDaily Granularity = iota
	GranularityWeekly
	GranularityMonthly
	GranularityYearly
)

func (g Granularity) String() string {
	switch g {
	case GranularityWeekly:
		return "weekly"
	case GranularityMonthly:
		return "monthly"
	case GranularityYearly:
		return "yearly"
	default:
		return "daily"
	}
}

func (g Granularity) Next() Granularity {
	return (g + 1) % 4
}

// AggregateStats buckets daily stats into the given granularity.
func AggregateStats(daily []DayStat, g Granularity) []DayStat {
	if g == GranularityDaily || len(daily) == 0 {
		return daily
	}

	buckets := make(map[time.Time]int)
	var keys []time.Time

	for _, s := range daily {
		key := bucketKey(s.Date, g)
		if _, exists := buckets[key]; !exists {
			keys = append(keys, key)
		}
		buckets[key] += s.Count
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	result := make([]DayStat, len(keys))
	for i, k := range keys {
		result[i] = DayStat{Date: k, Count: buckets[k]}
	}
	return result
}

func bucketKey(t time.Time, g Granularity) time.Time {
	switch g {
	case GranularityWeekly:
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := t.AddDate(0, 0, -(weekday - 1))
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
	case GranularityMonthly:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	case GranularityYearly:
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default:
		return t
	}
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// FileHealthInfo holds code health signals for a single file.
type FileHealthInfo struct {
	Path        string
	Lines       int
	AuthorCount int
	LastChanged time.Time
	Churn       int // number of commits that touched this file
}

// CollectFileLineCounts walks the HEAD tree and returns line counts per file.
func CollectFileLineCounts(repo *git.Repository) (map[string]int, error) {
	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	lineCounts := map[string]int{}
	tree.Files().ForEach(func(f *object.File) error {
		if f.Size > 1<<20 { // skip files >1MB
			return nil
		}
		reader, err := f.Reader()
		if err != nil {
			return nil
		}
		defer reader.Close()
		content, err := io.ReadAll(reader)
		if err != nil {
			return nil
		}
		lines := bytes.Count(content, []byte("\n"))
		if len(content) > 0 && content[len(content)-1] != '\n' {
			lines++ // count last line without trailing newline
		}
		lineCounts[f.Name] = lines
		return nil
	})

	return lineCounts, nil
}

// BuildHealthData combines line counts with commit-derived author/staleness data.
// If filtered is true, only files that appear in commits are included.
func BuildHealthData(lineCounts map[string]int, commits []CommitInfo, filtered bool) []FileHealthInfo {
	authors := map[string]map[string]bool{}
	lastChanged := map[string]time.Time{}
	churn := map[string]int{}
	for i := range commits {
		c := &commits[i]
		for _, f := range c.Files {
			if authors[f] == nil {
				authors[f] = map[string]bool{}
			}
			authors[f][c.Author] = true
			churn[f]++
			if c.Date.After(lastChanged[f]) {
				lastChanged[f] = c.Date
			}
		}
	}

	var result []FileHealthInfo
	if filtered {
		// Only include files that appear in the (filtered) commits.
		seen := map[string]bool{}
		for path := range authors {
			if seen[path] {
				continue
			}
			seen[path] = true
			result = append(result, FileHealthInfo{
				Path:        path,
				Lines:       lineCounts[path],
				AuthorCount: len(authors[path]),
				LastChanged: lastChanged[path],
				Churn:       churn[path],
			})
		}
	} else {
		for path, lines := range lineCounts {
			result = append(result, FileHealthInfo{
				Path:        path,
				Lines:       lines,
				AuthorCount: len(authors[path]),
				LastChanged: lastChanged[path],
				Churn:       churn[path],
			})
		}
	}
	return result
}

// BranchInfo holds metadata for a single branch.
type BranchInfo struct {
	Name       string
	IsCurrent  bool
	LastCommit time.Time
	Author     string
	Ahead      int // commits ahead of default branch
	Behind     int // commits behind default branch
	Stale      bool   // true if last commit is older than 90 days
	RemoteGone bool   // true if upstream remote branch no longer exists
	HasRemote  bool   // true if branch tracks a remote branch
}

// CollectBranches enumerates local branches and computes ahead/behind vs HEAD.
func CollectBranches(repo *git.Repository, mm *Mailmap) ([]BranchInfo, error) {
	headRef, err := repo.Head()
	if err != nil {
		return nil, err
	}
	headCommit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, err
	}
	_ = headCommit

	// Build a set of remote-tracking branch refs for quick lookup.
	remoteRefs := map[string]bool{}
	refs, err := repo.References()
	if err == nil {
		refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Name().IsRemote() {
				remoteRefs[ref.Name().String()] = true
			}
			return nil
		})
	}

	cfg, _ := repo.Config()
	now := time.Now()

	branches, err := repo.Branches()
	if err != nil {
		return nil, err
	}

	var result []BranchInfo
	err = branches.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		commit, err := repo.CommitObject(ref.Hash())
		if err != nil {
			return nil
		}

		authorName, _ := mm.Resolve(commit.Author.Name, commit.Author.Email)
		bi := BranchInfo{
			Name:       name,
			IsCurrent:  headRef.Name().IsBranch() && headRef.Name().Short() == name,
			LastCommit: commit.Author.When,
			Author:     authorName,
			Stale:      now.Sub(commit.Author.When) > 90*24*time.Hour,
		}

		if ref.Hash() != headRef.Hash() {
			ahead, behind := computeAheadBehind(repo, ref.Hash(), headRef.Hash())
			bi.Ahead = ahead
			bi.Behind = behind
		}

		// Check remote tracking status.
		if cfg != nil {
			if brCfg, ok := cfg.Branches[name]; ok {
				bi.HasRemote = true
				remoteRef := fmt.Sprintf("refs/remotes/%s/%s", brCfg.Remote, brCfg.Name)
				if brCfg.Name == "" {
					remoteRef = fmt.Sprintf("refs/remotes/%s/%s", brCfg.Remote, name)
				}
				if !remoteRefs[remoteRef] {
					bi.RemoteGone = true
				}
			}
		}

		result = append(result, bi)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// computeAheadBehind counts commits ahead/behind between branch and base.
func computeAheadBehind(repo *git.Repository, branchHash, baseHash plumbing.Hash) (ahead, behind int) {
	branchCommit, err := repo.CommitObject(branchHash)
	if err != nil {
		return 0, 0
	}
	baseCommit, err := repo.CommitObject(baseHash)
	if err != nil {
		return 0, 0
	}

	bases, err := branchCommit.MergeBase(baseCommit)
	if err != nil || len(bases) == 0 {
		return 0, 0
	}
	mergeBase := bases[0].Hash

	// Count ahead: walk from branch to merge base.
	iter, err := repo.Log(&git.LogOptions{From: branchHash})
	if err != nil {
		return 0, 0
	}
	iter.ForEach(func(c *object.Commit) error {
		if c.Hash == mergeBase {
			return io.EOF
		}
		ahead++
		return nil
	})

	// Count behind: walk from base to merge base.
	iter, err = repo.Log(&git.LogOptions{From: baseHash})
	if err != nil {
		return ahead, 0
	}
	iter.ForEach(func(c *object.Commit) error {
		if c.Hash == mergeBase {
			return io.EOF
		}
		behind++
		return nil
	})

	return ahead, behind
}

// TagInfo holds metadata for a single tag/release.
type TagInfo struct {
	Name       string
	Date       time.Time // tagger date for annotated, commit date for lightweight
	CommitHash string
	Annotated  bool
	Message    string // tag message (annotated only)
}

// CollectTags enumerates all tags and returns them sorted newest first.
func CollectTags(repo *git.Repository) ([]TagInfo, error) {
	iter, err := repo.Tags()
	if err != nil {
		return nil, err
	}

	var tags []TagInfo
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()

		// Try annotated tag first.
		tagObj, err := repo.TagObject(ref.Hash())
		if err == nil {
			commit, cerr := tagObj.Commit()
			commitHash := ""
			if cerr == nil {
				commitHash = commit.Hash.String()
			}
			tags = append(tags, TagInfo{
				Name:       name,
				Date:       tagObj.Tagger.When,
				CommitHash: commitHash,
				Annotated:  true,
				Message:    strings.TrimSpace(tagObj.Message),
			})
			return nil
		}

		// Lightweight tag — resolve to commit.
		commit, err := repo.CommitObject(ref.Hash())
		if err != nil {
			return nil
		}
		tags = append(tags, TagInfo{
			Name:       name,
			Date:       commit.Author.When,
			CommitHash: commit.Hash.String(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Date.After(tags[j].Date)
	})
	return tags, nil
}

// CountCommitsSince counts commits from HEAD that are after the given hash.
func CountCommitsSince(repo *git.Repository, sinceHash string) int {
	ref, err := repo.Head()
	if err != nil {
		return 0
	}

	count := 0
	iter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return 0
	}
	iter.ForEach(func(c *object.Commit) error {
		if c.Hash.String() == sinceHash {
			return io.EOF
		}
		count++
		return nil
	})
	return count
}
