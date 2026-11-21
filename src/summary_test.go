package main

import (
	"strings"
	"testing"
	"time"
)

func TestSummaryText(t *testing.T) {
	b := HistBucket{T: time.Date(2026, 11, 21, 14, 0, 0, 0, time.Local).Unix(), Pulls: 7200, Mystical: 2, Chroma: 5, Coins: 480000}
	prev := &HistBucket{Coins: 481250}
	title, text := summaryText(b, prev)
	if !strings.Contains(title, "14:00-15:00") {
		t.Fatal(title)
	}
	for _, want := range []string{"7,200 packs", "2.0 per second", "2 Mystical", "5 Chroma", "coins 480,000 (-1,250)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestHourlySummarySentOncePerHour(t *testing.T) {
	h := newTestHub(t, newFakeSite(100))
	defer h.Close()
	now := hourStart(time.Now())
	h.histMu.Lock()
	h.hist = []HistBucket{{T: now - 3600, Pulls: 500, Mystical: 1, Coins: 900}}
	h.histMu.Unlock()
	h.mu.Lock()
	h.sumHour = now - 3600 // pretend the app started an hour ago
	h.mu.Unlock()
	h.maybeSummary()
	h.maybeSummary()
	n := 0
	for {
		if _, _, ok := h.primary.takeBalloon(); !ok {
			break
		}
		n++
	}
	if n != 1 {
		t.Fatalf("summaries sent: %d", n)
	}
	// off: nothing
	h.update(func(c *Config) { c.HourlySummary = false })
	h.mu.Lock()
	h.sumHour = now - 3600
	h.mu.Unlock()
	h.maybeSummary()
	if _, _, ok := h.primary.takeBalloon(); ok {
		t.Fatal("sent while turned off")
	}
}
