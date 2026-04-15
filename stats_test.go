package main

import (
	"testing"
	"time"
)

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
