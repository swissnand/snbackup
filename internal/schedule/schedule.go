// Package schedule computes when jobs run: intervals, daily / weekly /
// monthly times, cron expressions, backup windows, pauses and jitter.
package schedule

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/swissnand/snbackup/internal/store"
)

// Cron is a parsed 5-field cron expression (minute hour dom month dow).
type Cron struct {
	min, hour, dom, mon, dow [64]bool
	domStar, dowStar         bool
}

var cronAliases = map[string]string{
	"@hourly": "0 * * * *", "@daily": "0 0 * * *", "@midnight": "0 0 * * *",
	"@weekly": "0 0 * * 0", "@monthly": "0 0 1 * *", "@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *",
}

var monNames = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
var dowNames = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

// ParseCron parses an expression such as "30 2 * * 1-5" or "@daily".
func ParseCron(expr string) (*Cron, error) {
	expr = strings.TrimSpace(strings.ToLower(expr))
	if a, ok := cronAliases[expr]; ok {
		expr = a
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return nil, errors.New("cron: 5 Felder erwartet (Minute Stunde Tag Monat Wochentag)")
	}
	c := &Cron{}
	specs := []struct {
		dst    *[64]bool
		lo, hi int
		names  map[string]int
	}{{&c.min, 0, 59, nil}, {&c.hour, 0, 23, nil}, {&c.dom, 1, 31, nil}, {&c.mon, 1, 12, monNames}, {&c.dow, 0, 7, dowNames}}
	for i, s := range specs {
		if err := parseField(f[i], s.lo, s.hi, s.names, s.dst); err != nil {
			return nil, fmt.Errorf("cron Feld %d: %w", i+1, err)
		}
	}
	if c.dow[7] {
		c.dow[0] = true
	}
	c.domStar = f[2] == "*" || f[2] == "?"
	c.dowStar = f[4] == "*" || f[4] == "?"
	return c, nil
}

func parseField(s string, lo, hi int, names map[string]int, dst *[64]bool) error {
	for _, part := range strings.Split(s, ",") {
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n <= 0 {
				return fmt.Errorf("ungültige Schrittweite %q", part)
			}
			step = n
			part = part[:i]
		}
		a, b := lo, hi
		if part != "*" && part != "?" {
			r := strings.SplitN(part, "-", 2)
			var err error
			if a, err = atoiName(r[0], names); err != nil {
				return err
			}
			b = a
			if len(r) == 2 {
				if b, err = atoiName(r[1], names); err != nil {
					return err
				}
			} else if step > 1 {
				b = hi
			}
		}
		if a < lo || b > hi || a > b {
			return fmt.Errorf("Wert ausserhalb %d-%d", lo, hi)
		}
		for v := a; v <= b; v += step {
			dst[v] = true
		}
	}
	return nil
}

func atoiName(s string, names map[string]int) (int, error) {
	if v, ok := names[s]; ok {
		return v, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("ungültiger Wert %q", s)
	}
	return v, nil
}

// Next returns the first matching minute strictly after t.
func (c *Cron) Next(t time.Time) time.Time {
	t = t.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if !c.mon[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		domOK, dowOK := c.dom[t.Day()], c.dow[int(t.Weekday())]
		dayOK := domOK && dowOK
		if !c.domStar && !c.dowStar {
			dayOK = domOK || dowOK
		}
		if !dayOK {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !c.hour[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !c.min[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

func parseHM(s string) (int, int, error) {
	p := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(p) != 2 {
		return 0, 0, fmt.Errorf("Zeit %q: HH:MM erwartet", s)
	}
	h, e1 := strconv.Atoi(p[0])
	m, e2 := strconv.Atoi(p[1])
	if e1 != nil || e2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("ungültige Zeit %q", s)
	}
	return h, m, nil
}

// Validate checks a schedule for errors.
func Validate(s store.Schedule) error {
	switch s.Kind {
	case "", "manual", "after":
		if s.Kind == "after" && s.AfterJob == "" {
			return errors.New("Bitte den vorausgehenden Job wählen")
		}
	case "interval":
		if s.EveryMin < 5 {
			return errors.New("Intervall muss mindestens 5 Minuten sein")
		}
	case "daily", "weekly", "monthly":
		if len(s.Times) == 0 {
			return errors.New("Mindestens eine Uhrzeit angeben")
		}
		for _, t := range s.Times {
			if _, _, err := parseHM(t); err != nil {
				return err
			}
		}
		if s.Kind == "weekly" && len(s.Weekdays) == 0 {
			return errors.New("Mindestens einen Wochentag wählen")
		}
		if s.Kind == "monthly" && len(s.MonthDays) == 0 {
			return errors.New("Mindestens einen Tag im Monat wählen")
		}
	case "cron":
		if _, err := ParseCron(s.Cron); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unbekannter Zeitplan %q", s.Kind)
	}
	for _, w := range []string{s.WindowStart, s.WindowEnd} {
		if w != "" {
			if _, _, err := parseHM(w); err != nil {
				return err
			}
		}
	}
	return nil
}

func lastDay(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

// rawNext ignores windows/pauses/jitter.
func rawNext(s store.Schedule, after time.Time, last *time.Time) time.Time {
	loc := after.Location()
	switch s.Kind {
	case "interval":
		base := after
		if last != nil && last.Add(time.Duration(s.EveryMin)*time.Minute).After(after) {
			return last.Add(time.Duration(s.EveryMin) * time.Minute)
		}
		if last == nil {
			return base.Add(time.Minute).Truncate(time.Minute)
		}
		return base.Add(time.Minute).Truncate(time.Minute)
	case "daily", "weekly", "monthly":
		var best time.Time
		for d := 0; d <= 62; d++ {
			day := time.Date(after.Year(), after.Month(), after.Day()+d, 0, 0, 0, 0, loc)
			ok := s.Kind == "daily"
			if s.Kind == "weekly" {
				for _, w := range s.Weekdays {
					if int(day.Weekday()) == w%7 {
						ok = true
					}
				}
			}
			if s.Kind == "monthly" {
				for _, md := range s.MonthDays {
					if md == day.Day() || (md == -1 && day.Day() == lastDay(day)) || (md > lastDay(day) && day.Day() == lastDay(day)) {
						ok = true
					}
				}
			}
			if !ok {
				continue
			}
			for _, tm := range s.Times {
				h, m, err := parseHM(tm)
				if err != nil {
					continue
				}
				c := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
				if c.After(after) && (best.IsZero() || c.Before(best)) {
					best = c
				}
			}
			if !best.IsZero() {
				return best
			}
		}
		return time.Time{}
	case "cron":
		c, err := ParseCron(s.Cron)
		if err != nil {
			return time.Time{}
		}
		return c.Next(after)
	}
	return time.Time{}
}

func inWindow(s store.Schedule, t time.Time) (bool, time.Time) {
	if s.WindowStart == "" || s.WindowEnd == "" {
		return true, t
	}
	sh, sm, _ := parseHM(s.WindowStart)
	eh, em, _ := parseHM(s.WindowEnd)
	cur := t.Hour()*60 + t.Minute()
	start, end := sh*60+sm, eh*60+em
	var ok bool
	if start <= end {
		ok = cur >= start && cur < end
	} else { // over midnight, e.g. 22:00-06:00
		ok = cur >= start || cur < end
	}
	if ok {
		return true, t
	}
	next := time.Date(t.Year(), t.Month(), t.Day(), sh, sm, 0, 0, t.Location())
	if !next.After(t) {
		next = next.AddDate(0, 0, 1)
	}
	return false, next
}

func paused(s store.Schedule, t time.Time) (bool, time.Time) {
	if s.PauseFrom == "" || s.PauseUntil == "" {
		return false, t
	}
	from, e1 := time.ParseInLocation("2006-01-02", s.PauseFrom, t.Location())
	until, e2 := time.ParseInLocation("2006-01-02", s.PauseUntil, t.Location())
	if e1 != nil || e2 != nil {
		return false, t
	}
	until = until.AddDate(0, 0, 1)
	if !t.Before(from) && t.Before(until) {
		return true, until
	}
	return false, t
}

// Jitter returns a deterministic per-job offset so many servers/jobs do not
// hit the storage at exactly the same second.
func Jitter(jobID string, maxMin int) time.Duration {
	if maxMin <= 0 {
		return 0
	}
	h := fnv.New32a()
	h.Write([]byte(jobID))
	return time.Duration(h.Sum32()%uint32(maxMin*60)) * time.Second
}

// Next returns the next run time of a job after `after` (zero = never).
func Next(j *store.Job, after time.Time) time.Time {
	s := j.Schedule
	if !j.Enabled || s.Kind == "" || s.Kind == "manual" || s.Kind == "after" {
		return time.Time{}
	}
	t := rawNext(s, after, j.LastRun)
	for i := 0; i < 500 && !t.IsZero(); i++ {
		if p, until := paused(s, t); p {
			t = rawNext(s, until.Add(-time.Minute), nil)
			continue
		}
		ok, next := inWindow(s, t)
		if ok {
			return t.Add(Jitter(j.ID, s.JitterMin))
		}
		if s.Kind == "interval" {
			t = next
			continue
		}
		t = rawNext(s, next.Add(-time.Minute), nil)
	}
	return time.Time{}
}

// Upcoming lists the next run times of a job within a duration.
func Upcoming(j *store.Job, from time.Time, d time.Duration, max int) []time.Time {
	var out []time.Time
	t := from
	jc := *j
	for len(out) < max {
		n := Next(&jc, t)
		if n.IsZero() || n.After(from.Add(d)) {
			break
		}
		out = append(out, n)
		nn := n
		jc.LastRun = &nn
		t = n
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Before(out[b]) })
	return out
}

// Describe returns a short German description of a schedule.
func Describe(s store.Schedule) string {
	days := []string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
	var d string
	switch s.Kind {
	case "", "manual":
		return "Manuell"
	case "interval":
		if s.EveryMin%60 == 0 {
			d = fmt.Sprintf("Alle %d h", s.EveryMin/60)
		} else {
			d = fmt.Sprintf("Alle %d min", s.EveryMin)
		}
	case "daily":
		d = "Täglich " + strings.Join(s.Times, ", ")
	case "weekly":
		var w []string
		for _, x := range s.Weekdays {
			w = append(w, days[x%7])
		}
		d = strings.Join(w, ", ") + " " + strings.Join(s.Times, ", ")
	case "monthly":
		var m []string
		for _, x := range s.MonthDays {
			if x == -1 {
				m = append(m, "letzter")
			} else {
				m = append(m, fmt.Sprintf("%d.", x))
			}
		}
		d = "Monatlich am " + strings.Join(m, ", ") + " " + strings.Join(s.Times, ", ")
	case "cron":
		d = "Cron: " + s.Cron
	case "after":
		d = "Nach anderem Job"
	}
	if s.WindowStart != "" && s.WindowEnd != "" {
		d += " · Fenster " + s.WindowStart + "–" + s.WindowEnd
	}
	return d
}
