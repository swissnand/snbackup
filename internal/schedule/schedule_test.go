package schedule

import (
	"testing"
	"time"

	"github.com/swissnand/snbackup/internal/store"
)

func at(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	return t
}

func TestNext(t *testing.T) {
	cases := []struct {
		s     store.Schedule
		after string
		want  string
	}{
		{store.Schedule{Kind: "daily", Times: []string{"22:00"}}, "2026-11-21 21:30", "2026-11-21 22:00"},
		{store.Schedule{Kind: "daily", Times: []string{"07:00", "12:30"}}, "2026-11-21 12:31", "2026-11-22 07:00"},
		{store.Schedule{Kind: "weekly", Times: []string{"06:30"}, Weekdays: []int{1, 3}}, "2026-11-21 10:00", "2026-11-23 06:30"},
		{store.Schedule{Kind: "monthly", Times: []string{"03:00"}, MonthDays: []int{-1}}, "2026-02-01 00:00", "2026-02-28 03:00"},
		{store.Schedule{Kind: "cron", Cron: "30 2 * * 1-5"}, "2026-11-21 10:00", "2026-11-23 02:30"},
		{store.Schedule{Kind: "cron", Cron: "0 */4 * * *"}, "2026-11-21 10:00", "2026-11-21 12:00"},
		{store.Schedule{Kind: "daily", Times: []string{"03:00"}, WindowStart: "22:00", WindowEnd: "06:00"}, "2026-11-21 10:00", "2026-11-22 03:00"},
		{store.Schedule{Kind: "daily", Times: []string{"12:00"}, WindowStart: "22:00", WindowEnd: "06:00"}, "2026-11-21 10:00", ""},
		{store.Schedule{Kind: "daily", Times: []string{"22:00"}, PauseFrom: "2026-11-21", PauseUntil: "2026-11-23"}, "2026-11-21 10:00", "2026-11-24 22:00"},
	}
	for i, c := range cases {
		j := &store.Job{ID: "x", Enabled: true, Schedule: c.s}
		got := Next(j, at(c.after))
		g := ""
		if !got.IsZero() {
			g = got.Format("2006-01-02 15:04")
		}
		if g != c.want {
			t.Errorf("case %d: got %q want %q", i, g, c.want)
		}
	}
}

func TestCronInvalid(t *testing.T) {
	for _, e := range []string{"", "* * *", "61 * * * *", "* 25 * * *"} {
		if _, err := ParseCron(e); err == nil {
			t.Errorf("%q should fail", e)
		}
	}
}
