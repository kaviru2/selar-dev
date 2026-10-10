package store

import (
	"testing"
	"time"
)

func TestPracticeStreakMatchesWorkerRule(t *testing.T) {
	day := func(s string) time.Time {
		v, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	today := day("2026-10-10")
	cases := []struct {
		name string
		days []string
		want int
	}{
		{"none", nil, 0},
		{"today only", []string{"2026-10-10"}, 1},
		{"yesterday counts before today's review", []string{"2026-10-09", "2026-10-08"}, 2},
		{"gap breaks the run", []string{"2026-10-10", "2026-10-08"}, 1},
		{"two days ago only", []string{"2026-10-08"}, 0},
		{"duplicates ignored", []string{"2026-10-10", "2026-10-10", "2026-10-09"}, 2},
	}
	for _, c := range cases {
		var days []time.Time
		for _, d := range c.days {
			days = append(days, day(d))
		}
		if got := PracticeStreak(days, today); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}
