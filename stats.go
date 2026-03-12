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

// CollectDailyStats walks the commit log and returns daily commit counts
// for the last `days` days, sorted ascending by date. Zero-count days are
// included so the chart has no gaps.
func CollectDailyStats(repo *git.Repository, days int) ([]DayStat, error) {
	now := time.Now()
	cutoff := truncateToDay(now).AddDate(0, 0, -days)

	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}

	iter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}

	counts := make(map[time.Time]int)
	err = iter.ForEach(func(c *object.Commit) error {
		d := truncateToDay(c.Author.When)
		if d.Before(cutoff) {
			return errStop
		}
		counts[d]++
		return nil
	})
	if err != nil && err != errStop {
		return nil, err
	}

	// Fill in all days from cutoff to today.
	var stats []DayStat
	for d := cutoff; !d.After(truncateToDay(now)); d = d.AddDate(0, 0, 1) {
		stats = append(stats, DayStat{Date: d, Count: counts[d]})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Date.Before(stats[j].Date)
	})

	return stats, nil
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

var errStop = &stopError{}

type stopError struct{}

func (e *stopError) Error() string { return "stop" }
