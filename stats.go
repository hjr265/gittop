package main

import (
	"bytes"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
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
func CollectCommits(repo *git.Repository, needFiles bool) ([]CommitInfo, error) {
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
		ci := CommitInfo{
			Hash:    c.Hash.String(),
			Author:  c.Author.Name,
			Email:   c.Author.Email,
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
	if err != nil {
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
	for i := range commits {
		c := &commits[i]
		for _, f := range c.Files {
			if authors[f] == nil {
				authors[f] = map[string]bool{}
			}
			authors[f][c.Author] = true
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
			})
		}
	} else {
		for path, lines := range lineCounts {
			result = append(result, FileHealthInfo{
				Path:        path,
				Lines:       lines,
				AuthorCount: len(authors[path]),
				LastChanged: lastChanged[path],
			})
		}
	}
	return result
}
