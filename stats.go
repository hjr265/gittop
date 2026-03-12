package main

import (
	"sort"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DayStat holds the commit count for a single calendar day.
type DayStat struct {
	Date  time.Time
	Count int
}

// CollectDailyStats walks the entire commit log reachable from HEAD and
// returns daily commit counts sorted ascending by date. Zero-count days
// are filled in so charts have no gaps.
func CollectDailyStats(repo *git.Repository) ([]DayStat, error) {
	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}

	counts := make(map[time.Time]int)
	var earliest time.Time
	err = iter.ForEach(func(c *object.Commit) error {
		d := truncateToDay(c.Author.When)
		counts[d]++
		if earliest.IsZero() || d.Before(earliest) {
			earliest = d
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if earliest.IsZero() {
		return nil, nil
	}

	// Fill in all days from earliest commit to today.
	now := truncateToDay(time.Now())
	var stats []DayStat
	for d := earliest; !d.After(now); d = d.AddDate(0, 0, 1) {
		stats = append(stats, DayStat{Date: d, Count: counts[d]})
	}

	return stats, nil
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
		// Week starts on Monday.
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

var errStop = &stopError{}

type stopError struct{}

func (e *stopError) Error() string { return "stop" }
