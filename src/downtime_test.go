package main

import (
	"testing"
	"time"
)

func TestDowntime(t *testing.T) {
	w, err := ParseDowntime("3:20pm-4pm, 23:00-1am")
	if err != nil || len(w) != 2 || w[0] != (DownWin{15*60 + 20, 16 * 60}) || w[1] != (DownWin{23 * 60, 60}) {
		t.Fatalf("%v %v", w, err)
	}
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.Local) }
	for _, c := range []struct {
		h, m int
		down bool
	}{{15, 19, false}, {15, 20, true}, {15, 59, true}, {16, 0, false}, {23, 30, true}, {0, 30, true}, {1, 0, false}, {12, 0, false}} {
		if d, _ := DownNow(w, at(c.h, c.m)); d != c.down {
			t.Errorf("%d:%02d down=%v want %v", c.h, c.m, d, c.down)
		}
	}
	if x, _ := ParseDowntime("3-4pm"); len(x) != 1 || x[0] != (DownWin{15 * 60, 16 * 60}) {
		t.Errorf("3-4pm: %v", x)
	}
	if _, err := ParseDowntime("nonsense"); err == nil {
		t.Error("want error")
	}
	if FormatDowntime(w) != "3:20pm-4pm, 11pm-1am" {
		t.Error(FormatDowntime(w))
	}
}
