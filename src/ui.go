package main

import (
	"image"
	"math"
	"time"
)

// A flat, crisp look: solid colours, thin borders, no gloss. Motion carries the polish:
// numbers count up, toggles slide, the tab underline glides, lists fade in, and the
// running dot pulses. Every animation is driven by UIState.Step (called by the window
// timer only while the window is visible) so the program idles at ~0% CPU in the tray.

const (
	winW = 960
	winH = 556
)

var (
	colBg      = RGB(10, 17, 34)
	colPanel   = RGB(13, 22, 44)
	colCard    = RGB(18, 30, 58)
	colCardHi  = RGB(24, 39, 74)
	colEdge    = RGB(34, 52, 94)
	colField   = RGB(9, 15, 32)
	colAccent  = RGB(59, 140, 255)
	colAccentD = RGB(40, 104, 214)
	colCyan    = RGB(92, 225, 255)
	colText    = RGB(236, 243, 255)
	colMuted   = RGB(140, 160, 204)
	colDim     = RGB(92, 112, 158)
	colGood    = RGB(66, 214, 132)
	colBad     = RGB(236, 84, 98)
	colBadD    = RGB(190, 56, 72)
	colWarn    = RGB(255, 196, 92)
)

var rarityColors = map[string]Col{
	"Common":    RGB(150, 170, 205),
	"Uncommon":  RGB(96, 214, 120),
	"Rare":      RGB(70, 160, 255),
	"Epic":      RGB(176, 116, 255),
	"Legendary": RGB(255, 170, 60),
	"Chroma":    RGB(80, 232, 224),
	"Mystical":  RGB(255, 100, 190),
	"Unique":    RGB(255, 225, 110),
}

type Hit struct {
	R  Rect
	ID string
}

type TextCmd struct {
	R     Rect
	S     string
	Size  int // pixel height
	Bold  bool
	Col   Col
	Align int  // 0 left, 1 centre, 2 right
	Clip  Rect // text outside this rectangle is cut off (zero = no clipping)
}

type Frame struct {
	Hits      []Hit
	Texts     []TextCmd
	Edits     map[string]Rect // native text boxes that are visible and where
	MaxScroll map[string]int
}

// UIState is everything that only affects drawing.
type UIState struct {
	Hover, Down, Focus string
	ShowPass           bool
	StartupOn, BootOn  bool
	Tab                int
	ThemeEdit          bool              // the custom theme editor is open inside Settings
	Settings           bool              // the Settings page is open in place of the tabs
	ThemeHex           map[string]string // hex text of the theme editor boxes (for the colour squares)
	SetMsg             string            // result line on the Settings page
	TriansView         int               // 0 = owned, 1 = missing
	HistRange          int               // History tab: 0 = 24 h, 1 = 7 days, 2 = 30 days
	Wall               time.Time         // clock for the History tab (zero = now); previews fix it
	Scroll             map[string]int    // "inv", "auc"
	DiagAt             float64           // when Copy diagnostics last worked (UIState.Now)
	Sel                string            // trian picked in the Auction tab
	Search             string            // lower-case inventory filter
	AucMsg             string
	ChromaImg          *image.RGBA
	ImgFor             func(name string) *image.RGBA

	Now    float64 // seconds, advanced by Step
	Settle bool    // jump every animation to its end (previews and tests)

	hov     map[string]float64
	disp    map[string]float64
	target  map[string]float64
	enter   float64 // 0..1, restarts when the tab changes
	lastTab int
	moving  bool
	running bool
}

func NewUIState() *UIState {
	return &UIState{
		Scroll: map[string]int{}, hov: map[string]float64{}, disp: map[string]float64{}, target: map[string]float64{},
		lastTab: -1,
	}
}

func easeOut(t float64) float64 {
	t = clampf(t, 0, 1)
	return 1 - (1-t)*(1-t)*(1-t)
}

// Step advances the animations by dt seconds.
func (st *UIState) Step(dt float64) {
	if dt <= 0 {
		return
	}
	if dt > 0.1 {
		dt = 0.1
	}
	st.Now += dt
	st.moving = false
	k := 1 - math.Exp(-dt*16)
	for id := range st.hov {
		want := 0.0
		if id == st.Hover || id == st.Down {
			want = 1
		}
		d := (want - st.hov[id]) * k
		if math.Abs(want-st.hov[id]) > 0.004 {
			st.hov[id] += d
			st.moving = true
		} else {
			st.hov[id] = want
		}
	}
	for key, want := range st.target {
		rate := 7.0
		if len(key) > 2 && (key[:2] == "tg" || key[:2] == "sg") {
			rate = 16
		}
		cur := st.disp[key]
		if math.Abs(want-cur) > 0.0005*math.Max(1, math.Abs(want)) {
			st.disp[key] += (want - cur) * (1 - math.Exp(-dt*rate))
			if math.Abs(want-st.disp[key]) < 0.0005*math.Max(1, math.Abs(want)) {
				st.disp[key] = want
			} else {
				st.moving = true
			}
		}
	}
	if st.enter < 1 {
		st.enter = math.Min(1, st.enter+dt/0.38)
		st.moving = true
	}
}

// Animating is true while something is still moving (or pulsing), so the window knows
// whether it needs to keep redrawing at full speed.
func (st *UIState) Animating() bool { return st.moving || st.running || st.enter < 1 }

// num returns a number that eases towards v (a count-up when it first appears).
func (st *UIState) num(key string, v float64) float64 {
	st.target[key] = v
	if st.Settle {
		st.disp[key] = v
		return v
	}
	return st.disp[key]
}

func (st *UIState) hv(id string) float64 {
	if _, ok := st.hov[id]; !ok {
		st.hov[id] = 0
	}
	if st.Settle {
		return 0
	}
	return st.hov[id]
}

// ---------- painter ----------

type painter struct {
	c  *Canvas
	f  *Frame
	st *UIState
	s  Snapshot
}

func (f *Frame) text(r Rect, s string, size int, bold bool, c Col, align int) {
	f.Texts = append(f.Texts, TextCmd{R: r, S: s, Size: size, Bold: bold, Col: c, Align: align})
}

func (f *Frame) hit(r Rect, id string) { f.Hits = append(f.Hits, Hit{r, id}) }

func (p *painter) text(r Rect, s string, size int, bold bool, c Col, align int) {
	var clip Rect
	if p.c.clip != (Rect{0, 0, p.c.W, p.c.H}) {
		clip = p.c.clip
		if r.Intersect(clip).Empty() {
			return
		}
	}
	p.f.Texts = append(p.f.Texts, TextCmd{r, s, size, bold, c, align, clip})
}

// fade dims everything drawn since the marks (canvas rectangle + texts) towards bg.
func (p *painter) fade(r Rect, bg Col, textFrom int, a float64) {
	if a >= 0.999 {
		return
	}
	p.c.SetClip(r)
	p.c.RoundRect(r.Inset(-2, -2), 0, bg.Op(1-a))
	p.c.ResetClip()
	for i := textFrom; i < len(p.f.Texts); i++ {
		p.f.Texts[i].Col.A = uint8(float64(p.f.Texts[i].Col.A) * a)
	}
}

func (p *painter) card(r Rect, fill Col) {
	p.c.RoundRect(r, 10, fill)
	p.c.RoundRectStroke(r, 10, 1, colEdge)
}

func (p *painter) label(x, y int, s string) {
	p.text(Rect{x, y, 200, 16}, s, 11, true, colAccent, 0)
}

func (p *painter) field(r Rect, id string) {
	foc := p.st.num("sg:f:"+id, b2f(p.st.Focus == id))
	p.c.RoundRect(r, 7, colField)
	p.c.RoundRectStroke(r, 7, 1, lerpCol(colEdge, colAccent, foc))
	if foc > 0.02 {
		p.c.RoundRectStroke(r.Inset(-1, -1), 8, 1, colAccent.Op(0.35*foc))
	}
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// button: kind 0 = filled accent colour, 1 = quiet outline.
func (p *painter) button(id string, r Rect, caption string, base Col, kind int, size int) {
	h := p.st.hv(id)
	press := 0.0
	if p.st.Down == id {
		press = 1
	}
	if kind == 0 {
		fill := lerpCol(base, RGB(255, 255, 255), 0.16*h)
		fill = lerpCol(fill, RGB(0, 0, 0), 0.22*press)
		p.c.RoundRect(r, 9, fill)
		p.text(r, caption, size, true, RGB(255, 255, 255), 1)
	} else {
		fill := lerpCol(colCard, colCardHi, h)
		fill = lerpCol(fill, colField, 0.5*press)
		p.c.RoundRect(r, 9, fill)
		p.c.RoundRectStroke(r, 9, 1, lerpCol(colEdge, colAccent, h))
		p.text(r, caption, size, false, lerpCol(colMuted, colText, h), 1)
	}
	p.f.hit(r, id)
}

func (p *painter) chip(id string, r Rect, label string, sel bool) {
	h := p.st.hv(id)
	s := p.st.num("sg:c:"+id, b2f(sel))
	fill := lerpCol(lerpCol(colField, colCardHi, h), colAccent, s)
	p.c.RoundRect(r, float64(r.H)/2, fill)
	if s < 0.99 {
		p.c.RoundRectStroke(r, float64(r.H)/2, 1, lerpCol(colEdge, colAccent, h).Op(1-s))
	}
	tc := lerpCol(lerpCol(colMuted, colText, h), RGB(255, 255, 255), s)
	p.text(r, label, 12, sel, tc, 1)
	p.f.hit(r, id)
}

// seg is a segmented control whose highlight slides between the choices.
func (p *painter) seg(prefix string, r Rect, labels []string, sel int) {
	p.c.RoundRect(r, 9, colField)
	p.c.RoundRectStroke(r, 9, 1, colEdge)
	w := float64(r.W-6) / float64(len(labels))
	pos := p.st.num("sg:seg:"+prefix, float64(sel))
	pill := Rect{r.X + 3 + int(pos*w+0.5), r.Y + 3, int(w), r.H - 6}
	if sel >= 0 {
		p.c.RoundRect(pill, 7, colAccent)
	}
	for i, l := range labels {
		id := prefix + ":" + itoaInt(i)
		cell := Rect{r.X + 3 + int(float64(i)*w), r.Y + 3, int(w), r.H - 6}
		col := colMuted
		if i == sel {
			col = RGB(255, 255, 255)
		} else if p.st.hv(id) > 0 {
			col = lerpCol(colMuted, colText, p.st.hv(id))
		}
		p.text(cell, l, 12, i == sel, col, 1)
		p.f.hit(cell, id)
	}
}

func (p *painter) toggle(id string, r Rect, label string, on bool) {
	h := p.st.hv(id)
	t := p.st.num("tg:"+id, b2f(on))
	p.c.RoundRect(r, 8, lerpCol(colField, colCardHi, h))
	p.c.RoundRectStroke(r, 8, 1, lerpCol(colEdge, colAccent, h*0.6))
	sw := Rect{r.X + 8, r.Y + (r.H-16)/2, 30, 16}
	p.c.RoundRect(sw, 8, lerpCol(colTrack, colAccent, t))
	kx := float64(sw.X) + 8 + 14*t
	p.c.Circle(kx, float64(sw.Y)+8, 6, RGB(255, 255, 255))
	p.text(Rect{r.X + 46, r.Y, r.W - 50, r.H}, label, 12, false, lerpCol(colMuted, colText, h), 0)
	p.f.hit(r, id)
}

func (p *painter) stepper(id string, r Rect, value string) {
	bw := 26
	minus := Rect{r.X, r.Y, bw, r.H}
	plus := Rect{r.X + r.W - bw, r.Y, bw, r.H}
	p.c.RoundRect(r, 8, colField)
	p.c.RoundRectStroke(r, 8, 1, colEdge)
	for _, b := range []struct {
		id string
		r  Rect
		s  string
	}{{id + "-", minus, "-"}, {id + "+", plus, "+"}} {
		h := p.st.hv(b.id)
		if h > 0.01 || p.st.Down == b.id {
			p.c.RoundRect(b.r.Inset(2, 2), 6, colAccent.Op(0.25+0.3*h))
		}
		p.text(b.r, b.s, 16, true, lerpCol(colMuted, colText, h), 1)
		p.f.hit(b.r, b.id)
	}
	p.text(Rect{r.X + bw, r.Y, r.W - 2*bw, r.H}, value, 13, true, colText, 1)
}

func itoaInt(n int) string { return fmtInt(n) }
