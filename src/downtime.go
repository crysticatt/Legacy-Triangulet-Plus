package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Downtime is a list of daily time ranges while the opener stays paused
// (for example the hours the site is down). Written as "3:20pm-4pm, 02:00-02:30".

// DownWin is one daily pause window in minutes after midnight. A window that
// ends before it starts runs over midnight.
type DownWin struct {
	Start int `json:"s"`
	End   int `json:"e"`
}

func parseClock(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	pm, am := strings.HasSuffix(s, "pm") || strings.HasSuffix(s, "p"), strings.HasSuffix(s, "am") || strings.HasSuffix(s, "a")
	s = strings.TrimRight(s, "apm ")
	h, m := s, "0"
	if i := strings.IndexAny(s, ":."); i >= 0 {
		h, m = s[:i], s[i+1:]
	}
	hh, e1 := strconv.Atoi(strings.TrimSpace(h))
	mm, e2 := strconv.Atoi(strings.TrimSpace(m))
	if e1 != nil || e2 != nil || mm < 0 || mm > 59 || hh < 0 {
		return 0, false
	}
	if am || pm {
		if hh < 1 || hh > 12 {
			return 0, false
		}
		hh %= 12
		if pm {
			hh += 12
		}
	}
	if hh > 24 || (hh == 24 && mm > 0) {
		return 0, false
	}
	return (hh*60 + mm) % 1440, true
}

// ParseDowntime reads "3:20pm-4pm, 02:00-02:30". An "am/pm" on the end time
// only applies to the start too when the start has none and fits ("3-4pm" is 3pm-4pm).
func ParseDowntime(s string) ([]DownWin, error) {
	var out []DownWin
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		part = strings.NewReplacer(" to ", "-", "–", "-", "—", "-").Replace(part)
		ab := strings.SplitN(part, "-", 2)
		if len(ab) != 2 {
			return nil, fmt.Errorf("can't read %q (use 3:20pm-4pm)", part)
		}
		a, b := strings.TrimSpace(ab[0]), strings.TrimSpace(ab[1])
		lb := strings.ToLower(b)
		if !strings.ContainsAny(strings.ToLower(a), "ap") && (strings.HasSuffix(lb, "pm") || strings.HasSuffix(lb, "am")) {
			suf := lb[len(lb)-2:]
			if x, ok := parseClock(a + suf); ok {
				if y, ok2 := parseClock(b); ok2 && x > y && suf == "pm" { // 11-1pm means 11am-1pm
					x, _ = parseClock(a + "am")
				}
				a = clockStr(x)
			}
		}
		x, ok1 := parseClock(a)
		y, ok2 := parseClock(b)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("can't read %q (use 3:20pm-4pm)", part)
		}
		if x == y {
			return nil, fmt.Errorf("%q starts and ends at the same time", part)
		}
		out = append(out, DownWin{x, y})
	}
	return out, nil
}

func clockStr(m int) string {
	h, mm := m/60%24, m%60
	suf := "am"
	if h >= 12 {
		suf = "pm"
	}
	h12 := h % 12
	if h12 == 0 {
		h12 = 12
	}
	if mm == 0 {
		return fmt.Sprintf("%d%s", h12, suf)
	}
	return fmt.Sprintf("%d:%02d%s", h12, mm, suf)
}

func FormatDowntime(w []DownWin) string {
	p := make([]string, len(w))
	for i, x := range w {
		p[i] = clockStr(x.Start) + "-" + clockStr(x.End)
	}
	return strings.Join(p, ", ")
}

func (w DownWin) covers(min int) bool {
	if w.Start < w.End {
		return min >= w.Start && min < w.End
	}
	return min >= w.Start || min < w.End
}

// DownNow reports whether now is inside a pause window and, if so, when it ends.
func DownNow(ws []DownWin, now time.Time) (bool, string) {
	min := now.Hour()*60 + now.Minute()
	for _, w := range ws {
		if w.covers(min) {
			return true, clockStr(w.End)
		}
	}
	return false, ""
}
