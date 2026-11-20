package main

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// measureStep is how long each concurrency level is tried.
var measureStep = 2500 * time.Millisecond

var measureLevels = []int{2, 4, 8, 16, 32, 64, 128, 256}

// SpeedPoint is the result for one concurrency level.
type SpeedPoint struct {
	Level  int
	Rate   float64 // successful requests per second
	ErrPct float64
}

// settingsFor turns a number of simultaneous requests into Workers x At once.
func settingsFor(level int) (workers, inflight int) {
	workers = (level + 7) / 8
	if workers < 1 {
		workers = 1
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}
	inflight = (level + workers - 1) / workers
	if inflight < 1 {
		inflight = 1
	}
	if inflight > maxInFlight {
		inflight = maxInFlight
	}
	return
}

// bestLevel picks the smallest level that reaches 90% of the best clean rate: more requests
// than that only add load without adding speed.
func bestLevel(pts []SpeedPoint) (SpeedPoint, bool) {
	best := 0.0
	for _, p := range pts {
		if p.ErrPct < 1 && p.Rate > best {
			best = p.Rate
		}
	}
	if best <= 0 {
		return SpeedPoint{}, false
	}
	for _, p := range pts {
		if p.ErrPct < 1 && p.Rate >= 0.9*best {
			return p, true
		}
	}
	return SpeedPoint{}, false
}

// probe hammers a read-only endpoint with `level` simultaneous requests for d. It never opens a
// capsule, so it costs no coins.
func (a *App) probe(level int, d time.Duration) SpeedPoint {
	var ok, bad int64
	end := time.Now().Add(d)
	var wg sync.WaitGroup
	for i := 0; i < level; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(end) {
				resp, err := a.do("GET", "/data/user", nil)
				if err != nil {
					atomic.AddInt64(&bad, 1)
					time.Sleep(5 * time.Millisecond)
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					atomic.AddInt64(&bad, 1)
				} else {
					atomic.AddInt64(&ok, 1)
				}
			}
		}()
	}
	wg.Wait()
	tot := float64(ok + bad)
	p := SpeedPoint{Level: level, Rate: float64(ok) / d.Seconds()}
	if tot > 0 {
		p.ErrPct = float64(bad) / tot * 100
	}
	return p
}

// MeasureLimits finds how many simultaneous requests the site (and this connection) handles
// well, then sets Workers and At once to match. It only reads, so it spends no coins.
func (h *Hub) MeasureLimits() {
	h.mu.Lock()
	switch {
	case h.speedBusy:
		h.mu.Unlock()
		return
	case h.want:
		h.speedMsg = "Stop opening first, then test."
		h.mu.Unlock()
		return
	}
	h.speedBusy = true
	h.speedMsg = "Testing..."
	h.mu.Unlock()

	set := func(msg string, busy bool) {
		h.mu.Lock()
		h.speedMsg, h.speedBusy = msg, busy
		h.mu.Unlock()
	}
	h.bg(func() {
		p := h.primary
		if err := p.EnsureToken(); err != nil || !p.hasToken() {
			set("Log in first.", false)
			return
		}
		p.Warm(8)
		var pts []SpeedPoint
		bestRate := 0.0
		weak := 0
		for _, lv := range measureLevels {
			select {
			case <-h.quit:
				set("", false)
				return
			default:
			}
			if h.wanted() {
				set("Cancelled: opening started.", false)
				return
			}
			set(fmt.Sprintf("Testing %d at once...", lv), true)
			pt := p.probe(lv, measureStep)
			pts = append(pts, pt)
			if pt.ErrPct >= 5 {
				break // the site is pushing back: stop climbing
			}
			if pt.Rate > bestRate {
				bestRate, weak = pt.Rate, 0
			} else if pt.Rate < 1.05*bestRate {
				weak++
				if weak >= 2 {
					break // more requests no longer give more speed
				}
			}
		}
		best, ok := bestLevel(pts)
		if !ok {
			set("Test failed: the site did not answer cleanly.", false)
			return
		}
		w, f := settingsFor(best.Level)
		h.SetWorkers(w)
		h.SetInFlight(f)
		set(fmt.Sprintf("Set %d workers x %d (~%s req/s)", w, f, fmtInt(int(best.Rate))), false)
	})
}
