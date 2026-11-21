package main

import "testing"

func TestAuctionPickable(t *testing.T) {
	s := sampleSnapshot()
	st := NewUIState()
	st.Tab = 5
	c := NewCanvas(winW, winH)
	f := DrawScene(c, s, st)
	n := 0
	for _, h := range f.Hits {
		if len(h.ID) > 5 && h.ID[:5] == "pick:" {
			n++
		}
	}
	// a click in the middle of the first row must land on the row, not on the scroll area
	var row Rect
	for _, h := range f.Hits {
		if len(h.ID) > 5 && h.ID[:5] == "pick:" {
			row = h.R
			break
		}
	}
	top := ""
	for i := len(f.Hits) - 1; i >= 0; i-- {
		if f.Hits[i].R.In(row.X+row.W/2, row.Y+row.H/2) {
			top = f.Hits[i].ID
			break
		}
	}
	if len(top) < 5 || top[:5] != "pick:" {
		t.Fatalf("topmost hit over a row is %q, want pick:*", top)
	}
	if n == 0 {
		t.Fatalf("no pick hits on auction tab")
	}
}
