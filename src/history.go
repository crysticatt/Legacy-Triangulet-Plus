package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// HistBucket is one hour of activity. It is kept between runs in history.json so the History
// tab can show trends over days and weeks.
type HistBucket struct {
	T        int64 `json:"t"`     // start of the hour (unix seconds)
	Pulls    int   `json:"pulls"` // capsules opened
	Mystical int   `json:"myst"`
	Chroma   int   `json:"chroma"`
	Coins    int   `json:"coins"` // last known balance in this hour (-1 = unknown)
}

const histKeepDays = 90

func histPath(dir string) string { return filepath.Join(dir, "history.json") }

func hourStart(t time.Time) int64 { return t.Truncate(time.Hour).Unix() }

func (h *Hub) loadHistory() {
	b, err := os.ReadFile(histPath(h.primary.dir))
	if err != nil {
		return
	}
	var list []HistBucket
	if json.Unmarshal(b, &list) == nil {
		sort.Slice(list, func(i, j int) bool { return list[i].T < list[j].T })
		h.hist = list
	}
}

func (h *Hub) saveHistory() {
	h.histMu.Lock()
	b, _ := json.Marshal(h.hist)
	h.histDirty = false
	h.histMu.Unlock()
	_ = os.WriteFile(histPath(h.primary.dir), b, 0o600)
}

// histBucketLocked returns the bucket for now's hour, creating it (and dropping old ones).
func (h *Hub) histBucketLocked(now time.Time) *HistBucket {
	t := hourStart(now)
	if n := len(h.hist); n > 0 && h.hist[n-1].T == t {
		return &h.hist[n-1]
	}
	h.hist = append(h.hist, HistBucket{T: t, Coins: -1})
	cut := now.AddDate(0, 0, -histKeepDays).Unix()
	i := 0
	for i < len(h.hist)-1 && h.hist[i].T < cut {
		i++
	}
	h.hist = h.hist[i:]
	return &h.hist[len(h.hist)-1]
}

// recordHist counts one pull in the current hour.
func (h *Hub) recordHist(rarity string) {
	h.histMu.Lock()
	b := h.histBucketLocked(time.Now())
	b.Pulls++
	switch rarity {
	case "Mystical":
		b.Mystical++
	case "Chroma":
		b.Chroma++
	}
	h.histDirty = true
	h.histMu.Unlock()
}

// recordCoins notes the current balance (called every few seconds from the supervisor).
func (h *Hub) recordCoins(coins int64) {
	if coins < 0 {
		return
	}
	h.histMu.Lock()
	b := h.histBucketLocked(time.Now())
	if b.Coins != int(coins) {
		b.Coins = int(coins)
		h.histDirty = true
	}
	h.histMu.Unlock()
}

// flushHistory writes history.json if anything changed.
func (h *Hub) flushHistory() {
	h.histMu.Lock()
	dirty := h.histDirty
	h.histMu.Unlock()
	if dirty {
		h.saveHistory()
	}
}

func (h *Hub) historySnapshot() []HistBucket {
	h.histMu.Lock()
	defer h.histMu.Unlock()
	cut := time.Now().AddDate(0, 0, -31).Unix()
	i := sort.Search(len(h.hist), func(i int) bool { return h.hist[i].T >= cut })
	return append([]HistBucket(nil), h.hist[i:]...)
}

// HistPoint is one bar or point of a chart.
type HistPoint struct {
	Label    string
	Pulls    int
	Mystical int
	Chroma   int
	Coins    int // -1 = unknown
}

// histRanges: 0 = last 24 hours (hourly), 1 = last 7 days (daily), 2 = last 30 days (daily).
var histRangeNames = []string{"24 h", "7 days", "30 days"}

// histSeries groups buckets into points ending at `now`. Empty hours and days are kept as
// zeros so gaps show as gaps. Missing balances carry forward from the previous known one.
func histSeries(hist []HistBucket, now time.Time, rng int) []HistPoint {
	var n int
	var step time.Duration
	hourly := rng == 0
	switch rng {
	case 0:
		n, step = 24, time.Hour
	case 1:
		n = 7
	default:
		n = 30
	}
	pts := make([]HistPoint, n)
	var start time.Time
	if hourly {
		start = now.Truncate(time.Hour).Add(-time.Duration(n-1) * step)
	} else {
		y, m, d := now.Date()
		start = time.Date(y, m, d, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(n - 1))
	}
	slot := func(i int) time.Time {
		if hourly {
			return start.Add(time.Duration(i) * step)
		}
		return start.AddDate(0, 0, i)
	}
	for i := range pts {
		pts[i].Coins = -1
		if hourly {
			pts[i].Label = slot(i).Format("15:04")
		} else {
			pts[i].Label = slot(i).Format("Jan 2")
		}
	}
	carry := -1
	for _, b := range hist {
		bt := time.Unix(b.T, 0).In(now.Location())
		if bt.Before(start) {
			if b.Coins >= 0 {
				carry = b.Coins
			}
			continue
		}
		var i int
		if hourly {
			i = int(bt.Sub(start) / step)
		} else {
			y, m, d := bt.Date()
			day := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
			i = int(day.Sub(start).Round(24*time.Hour) / (24 * time.Hour))
		}
		if i < 0 || i >= n {
			continue
		}
		pts[i].Pulls += b.Pulls
		pts[i].Mystical += b.Mystical
		pts[i].Chroma += b.Chroma
		if b.Coins >= 0 {
			pts[i].Coins = b.Coins // buckets are in time order, so the latest wins
		}
	}
	for i := range pts {
		if pts[i].Coins >= 0 {
			carry = pts[i].Coins
		} else {
			pts[i].Coins = carry
		}
	}
	return pts
}
