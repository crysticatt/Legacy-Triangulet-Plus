package main

import "fmt"

// The tray icon's pop-up: a small flat card instead of the plain Windows menu.

const (
	popW = 264
	popH = 288
)

type PopupState struct {
	Hover string
	Now   float64
	hov   map[string]float64
}

func (ps *PopupState) Step(dt float64) {
	if ps.hov == nil {
		ps.hov = map[string]float64{}
	}
	ps.Now += dt
	for id := range ps.hov {
		want := 0.0
		if id == ps.Hover {
			want = 1
		}
		ps.hov[id] += (want - ps.hov[id]) * 0.45
	}
}

func (ps *PopupState) h(id string) float64 {
	if ps.hov == nil {
		ps.hov = map[string]float64{}
	}
	if _, ok := ps.hov[id]; !ok {
		ps.hov[id] = 0
	}
	return ps.hov[id]
}

func DrawPopup(c *Canvas, s Snapshot, ps *PopupState) Frame {
	f := Frame{Edits: map[string]Rect{}, MaxScroll: map[string]int{}}
	c.ResetClip()
	c.Gradient(Rect{0, 0, popW, popH}, colPanel, colPanel)
	c.RoundRectStroke(Rect{0, 0, popW, popH}, 12, 1, colEdge)

	c.Image(appLogo(), Rect{14, 14, 34, 34}, 9)
	f.text(Rect{56, 12, 190, 22}, "Triangulet Opener", 15, true, colText, 0)
	state, sc := "Idle", colMuted
	if s.Running {
		state, sc = fmt.Sprintf("Opening - %d/%d workers", s.WorkersRunning, s.Workers), colGood
		c.Circle(61, 43, 3.5, colGood)
		f.text(Rect{70, 34, 180, 18}, state, 12, false, sc, 0)
	} else {
		c.Circle(61, 43, 3.5, colDim)
		f.text(Rect{70, 34, 180, 18}, state, 12, false, sc, 0)
	}
	if s.Notice != "" {
		f.text(Rect{14, 56, popW - 28, 16}, s.Notice, 11, false, colWarn, 0)
	}

	// three numbers
	box := Rect{12, 76, popW - 24, 58}
	c.RoundRect(box, 9, colCard)
	cw := box.W / 3
	vals := []struct{ l, v string }{
		{"OPENED", fmtInt(s.Opened)},
		{"PER MIN", fmtInt(int(s.PerMin))},
		{"COINS", map[bool]string{true: fmtInt(s.Coins), false: "-"}[s.HasAcct]},
	}
	for i, v := range vals {
		f.text(Rect{box.X + i*cw, box.Y + 8, cw, 14}, v.l, 9, true, colMuted, 1)
		f.text(Rect{box.X + i*cw, box.Y + 24, cw, 26}, v.v, 16, true, colText, 1)
	}
	mode := map[string]string{"fixed": "Fixed capsule", "advance": "Next on Mystical", "hunt": "Mystical hunt"}[s.Mode]
	f.text(Rect{14, 140, popW - 28, 16}, fmt.Sprintf("%s  -  %s", mode, s.Capsule), 11, false, colDim, 0)

	btn := func(id string, y int, label string, fill Col) {
		r := Rect{12, y, popW - 24, 34}
		h := ps.h(id)
		c.RoundRect(r, 9, lerpCol(fill, RGB(255, 255, 255), 0.14*h))
		f.text(r, label, 13, true, colText, 1)
		f.hit(r, id)
	}
	if s.Running {
		btn("toggle", 162, "Stop opening", colBadD)
	} else {
		btn("toggle", 162, "Start opening", colAccent)
	}
	btn("show", 202, "Open window", colCardHi)
	btn("quit", 242, "Quit", colCard)
	return f
}
