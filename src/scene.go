package main

import (
	"fmt"
	"math"
	"strings"
)

const appVersion = "43"

var tabNames = []string{"Trians", "Stats", "Hunt", "Log", "History", "Auction", "Luck", "Wishlist"}

func modeIndex(m string) int {
	switch m {
	case "fixed":
		return 0
	case "hunt":
		return 2
	case "least":
		return 3
	case "wish":
		return 4
	}
	return 1
}

// DrawScene paints the whole window (everything except text) and returns what the
// window needs to do the rest. It has no Windows code in it, so it renders anywhere.
func DrawScene(c *Canvas, s Snapshot, st *UIState) Frame {
	f := Frame{Edits: map[string]Rect{}, MaxScroll: map[string]int{}}
	p := &painter{c: c, f: &f, st: st, s: s}
	st.running = s.Running

	c.ResetClip()
	c.Gradient(Rect{0, 0, winW, winH}, colBg, colBg)
	c.Gradient(Rect{0, 0, 372, winH}, colPanel, colPanel)
	c.Line(372, 0, 372, winH, 1, colEdge.Op(0.7))

	p.header()
	p.account()
	p.modeCard()
	p.speedCard()
	p.startRow()

	p.tiles()
	p.tabs()
	return f
}

func (p *painter) header() {
	c, s, st := p.c, p.s, p.st
	c.Image(appLogo(), Rect{16, 14, 40, 40}, 10)
	p.text(Rect{66, 12, 200, 24}, "Triangulet Plus", 18, true, colText, 0)
	statusCol, statusTxt := colMuted, s.StatusLine()
	switch {
	case s.Notice != "":
		statusCol, statusTxt = colWarn, s.Notice
	case s.Info != "" && !s.Running:
		statusTxt = s.Info
	}
	p.text(Rect{66, 36, 200, 18}, statusTxt, 12, false, statusCol, 0)

	pill := Rect{268, 22, 88, 26}
	on := st.num("sg:run", b2f(s.Running))
	c.RoundRect(pill, 13, lerpCol(colCard, RGB(18, 52, 44), on))
	c.RoundRectStroke(pill, 13, 1, lerpCol(colEdge, colGood, on))
	dot := lerpCol(colDim, colGood, on)
	cx, cy := float64(pill.X)+15, float64(pill.Y)+13
	if s.Running {
		ph := math.Mod(st.Now, 1.4) / 1.4
		c.Circle(cx, cy, 5+7*ph, colGood.Op(0.35*(1-ph)))
	}
	c.Circle(cx, cy, 4.5, dot)
	label, lc := "Idle", colMuted
	if s.Running {
		label, lc = "Opening", colGood
		if s.Down {
			label, lc = "Paused", colWarn
		}
	}
	p.text(Rect{pill.X + 26, pill.Y, pill.W - 28, pill.H}, label, 12, true, lc, 0)
	if s.UpdateVer != "" || s.UpdateMsg != "" {
		txt := "Update v" + s.UpdateVer
		if s.UpdateBusy {
			txt = "Updating..."
		}
		if s.UpdateMsg != "" && !s.UpdateBusy {
			txt = "Retry update"
		}
		p.button("update", Rect{pill.X - 4, 51, pill.W + 4, 15}, txt, colGood.Op(0.85), 0, 10)
		if s.UpdateMsg != "" && s.Notice == "" {
			p.text(Rect{66, 36, 200, 18}, s.UpdateMsg, 12, false, colWarn, 0)
		}
	}
}

func (p *painter) account() {
	s, st := p.s, p.st
	r := Rect{16, 66, 340, 96}
	p.card(r, colCard)
	p.label(r.X+12, r.Y+8, "ACCOUNT")
	desc := "not logged in"
	switch {
	case s.TokenDesc != "":
		desc = s.TokenDesc
	case s.HasCreds:
		desc = "token is fetched automatically"
	}
	// switcher: < > + - (previous / next saved login, add a new one, remove this one)
	bw, bx := 22, r.X+r.W-12
	for i, b := range []struct{ id, txt string }{{"acct-", "-"}, {"acctnew", "+"}, {"acct>", ">"}, {"acct<", "<"}} {
		rc := Rect{bx - (i+1)*bw - i*3, r.Y + 5, bw, 20}
		on := b.id == "acct<" || b.id == "acct>"
		if on && len(s.AcctNames) < 2 {
			p.c.RoundRect(rc, 6, colCard)
			p.text(rc, b.txt, 12, false, colDim.Op(0.5), 1)
			continue
		}
		if b.id == "acct-" && s.AcctIdx < 0 {
			p.c.RoundRect(rc, 6, colCard)
			p.text(rc, b.txt, 12, false, colDim.Op(0.5), 1)
			continue
		}
		p.button(b.id, rc, b.txt, colCard, 1, 12)
	}
	if len(s.AcctNames) > 1 && s.AcctIdx >= 0 {
		desc = fmt.Sprintf("login %d of %d", s.AcctIdx+1, len(s.AcctNames))
	}
	p.text(Rect{r.X + 70, r.Y + 8, r.W - 70 - 12 - 4*bw - 12, 16}, desc, 11, false, colDim, 2)
	uf := Rect{r.X + 12, r.Y + 28, 150, 30}
	pf := Rect{uf.X + 158, uf.Y, 158, 30}
	p.field(uf, "user")
	p.field(pf, "pass")
	p.f.Edits["user"] = Rect{uf.X + 9, uf.Y + 7, uf.W - 18, 16}
	p.f.Edits["pass"] = Rect{pf.X + 9, pf.Y + 7, pf.W - 18 - 36, 16}
	show := Rect{pf.X + pf.W - 40, pf.Y + 3, 36, 24}
	if h := st.hv("show"); h > 0.01 {
		p.c.RoundRect(show, 6, colAccent.Op(0.25*h))
	}
	p.text(show, map[bool]string{false: "show", true: "hide"}[st.ShowPass], 11, false, colMuted, 1)
	p.f.hit(show, "show")
	p.button("login", Rect{r.X + 12, r.Y + 64, 316, 24}, "Save and log in", colAccent, 0, 13)
}

func (p *painter) modeCard() {
	s, st := p.s, p.st
	r := Rect{16, 170, 340, 172}
	p.card(r, colCard)
	p.label(r.X+12, r.Y+8, "MODE")
	rf := Rect{r.X + r.W - 12 - 60, r.Y + 5, 60, 20}
	p.button("refresh", rf, "refresh", colCard, 1, 11)
	p.seg("mode", Rect{r.X + 12, r.Y + 28, 316, 30}, []string{"Fixed", "Next", "Hunt", "Least", "Wish"}, modeIndex(s.Mode))

	area := Rect{r.X + 12, r.Y + 68, 316, 96}
	if s.Mode == "least" || s.Mode == "wish" {
		what := "Go for the trian you own the fewest of (random if tied)"
		if s.Mode == "wish" {
			what = "Go for the wishlist trian you own the fewest of"
		}
		p.text(Rect{area.X, area.Y, area.W, 16}, what, 12, false, colMuted, 0)
		p.text(Rect{area.X, area.Y + 26, area.W, 16}, "Now going for", 11, true, colAccent, 0)
		now := s.Target
		switch {
		case now == "" && s.Mode == "wish" && len(s.Wishlist) == 0:
			now = "Star some trians in the Wishlist tab"
		case now == "":
			now = "Log in to load your trians"
		}
		p.text(Rect{area.X, area.Y + 42, area.W, 18}, now, 14, true, colText, 0)
		return
	}
	if s.Mode == "hunt" {
		p.text(Rect{area.X, area.Y, area.W, 16}, "Aim the workers at the best Mystical odds", 12, false, colMuted, 0)
		p.seg("spread", Rect{area.X, area.Y + 22, area.W, 28}, []string{"Focus", "Split 2", "Split 3", "Split 4"}, mini(maxi(s.HuntSpread, 1), 4)-1)
		var parts []string
		for _, h := range s.Hunt {
			if h.Workers > 0 {
				if h.Known {
					parts = append(parts, fmt.Sprintf("%s %s%%", h.Capsule, trimFloat(h.Odds)))
				} else {
					parts = append(parts, h.Capsule)
				}
			}
		}
		now := "Log in to rank the capsules"
		if len(parts) > 0 {
			now = strings.Join(parts, "   ·   ")
		}
		p.text(Rect{area.X, area.Y + 58, area.W, 16}, "Now hunting", 11, true, colAccent, 0)
		p.text(Rect{area.X, area.Y + 74, area.W, 18}, now, 12, false, colText, 0)
		return
	}
	names := s.Capsules
	if len(names) == 0 {
		names = builtinCapsules
	}
	x, y := area.X, area.Y
	cw := (area.W - 3*6) / 4
	for i, n := range names {
		if i > 0 && i%4 == 0 {
			x, y = area.X, y+32
		}
		p.chip("cap:"+n, Rect{x, y, cw, 26}, shorten(n, 9), n == s.Capsule)
		x += cw + 6
	}
	_ = st
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (p *painter) speedCard() {
	s, st := p.s, p.st
	r := Rect{16, 350, 340, 134}
	p.card(r, colCard)
	p.label(r.X+12, r.Y+8, "SPEED")
	colW := 100
	x0 := r.X + 12
	p.text(Rect{x0, r.Y + 26, colW, 14}, "Delay (ms)", 11, false, colMuted, 0)
	df := Rect{x0, r.Y + 42, colW, 28}
	p.field(df, "delay")
	p.f.Edits["delay"] = Rect{df.X + 9, df.Y + 6, df.W - 18, 16}
	p.text(Rect{x0 + colW + 8, r.Y + 26, colW, 14}, "Workers", 11, false, colMuted, 0)
	p.stepper("workers", Rect{x0 + colW + 8, r.Y + 42, colW, 28}, fmt.Sprintf("%d", s.Workers))
	p.text(Rect{x0 + 2*(colW+8), r.Y + 26, colW, 14}, "At once", 11, false, colMuted, 0)
	p.stepper("inflight", Rect{x0 + 2*(colW+8), r.Y + 42, colW, 28}, fmt.Sprintf("%d", s.InFlight))
	lanes := s.Workers * s.InFlight
	line := fmt.Sprintf("%d in flight  ·  %d/%d running", lanes, s.WorkersRunning, s.Workers)
	lc := colDim
	if s.SpeedMsg != "" {
		line, lc = s.SpeedMsg, colMuted
	}
	p.text(Rect{x0, r.Y + 74, 226, 16}, line, 11, false, lc, 0)
	label := "Test limits"
	if s.SpeedBusy {
		label = "Testing..."
	}
	p.button("speedtest", Rect{x0 + 316 - 82, r.Y + 72, 82, 20}, label, colCard, 1, 11)
	tw := 154
	p.toggle("startup", Rect{x0, r.Y + 98, tw, 26}, "Run at startup", st.StartupOn)
	p.toggle("boot", Rect{x0 + tw + 8, r.Y + 98, tw, 26}, "Before sign-in", st.BootOn)
}

func (p *painter) startRow() {
	s := p.s
	if s.Running {
		p.button("start", Rect{16, 492, 140, 36}, "Stop opening", colBadD, 0, 14)
	} else {
		p.button("start", Rect{16, 492, 140, 36}, "Start opening", colAccent, 0, 14)
	}
	p.button("hide", Rect{164, 492, 88, 36}, "Hide to tray", colCard, 1, 12)
	p.button("settings", Rect{260, 492, 96, 36}, "Settings", lerpCol(colCard, colAccent, b2f(p.st.Settings)*0.5), 1, 12)
}

func (p *painter) tile(r Rect, label string, value float64, sub string, valueText string) {
	p.card(r, colCard)
	p.text(Rect{r.X + 12, r.Y + 8, r.W - 24, 14}, label, 10, true, colMuted, 0)
	v := valueText
	if v == "" {
		v = fmtInt(int(p.st.num("n:"+label, value) + 0.5))
	}
	p.text(Rect{r.X + 12, r.Y + 22, r.W - 24, 28}, v, 22, true, colText, 0)
	p.text(Rect{r.X + 12, r.Y + 49, r.W - 24, 14}, sub, 11, false, colDim, 0)
}

func (p *painter) tiles() {
	s := p.s
	w := (556 - 3*8) / 4
	x := 388
	y, h := 14, 68
	rect := func(i int) Rect { return Rect{x + i*(w+8), y, w, h} }

	if s.HasAcct {
		p.tile(rect(0), "COINS", float64(s.Coins), fmt.Sprintf("%s packs left", fmtInt(s.PacksLeft)), "")
		p.tile(rect(1), "CAPSULES OPENED", float64(s.AcctOpened), fmt.Sprintf("+%s this session", fmtInt(s.Opened)), "")
	} else {
		p.tile(rect(0), "COINS", 0, "log in to load", "-")
		p.tile(rect(1), "CAPSULES OPENED", float64(s.Opened), "this session", "")
	}
	p.tile(rect(2), "PER MINUTE", s.PerMin, fmt.Sprintf("%s per hour", fmtInt(int(s.PerMin*60))), "")
	sub := "none yet"
	if s.Mystical > 0 {
		sub = fmt.Sprintf("%s since last", fmtInt(s.SinceMystical))
	}
	p.tile(rect(3), "MYSTICAL", float64(s.Mystical), sub, "")
}

func (p *painter) tabs() {
	c, s, st := p.c, p.s, p.st
	bar := Rect{388, 90, 556, 32}
	tw := 69
	pos := st.num("sg:tab", float64(st.Tab))
	for i, n := range tabNames {
		id := "tab:" + itoaInt(i)
		r := Rect{bar.X + i*tw, bar.Y, tw, bar.H - 2}
		col := lerpCol(colMuted, colText, st.hv(id))
		if i == st.Tab && !st.Settings {
			col = colText
		}
		p.text(r, n, 13, i == st.Tab && !st.Settings, col, 1)
		p.f.hit(r, id)
	}
	c.Line(float64(bar.X), float64(bar.Y+bar.H), float64(bar.X+bar.W), float64(bar.Y+bar.H), 1, colEdge)
	ux := float64(bar.X) + pos*float64(tw) + 16
	if !st.Settings {
		c.RoundRect(Rect{int(ux), bar.Y + bar.H - 2, tw - 32, 3}, 1.5, colAccent)
	}
	eff := st.Tab
	if st.Settings {
		eff = 99
	}

	// content
	if st.lastTab != eff {
		st.lastTab = eff
		st.enter = 0
		st.disp["sg:graph"] = 0
		for k := range st.disp {
			if strings.HasPrefix(k, "bar:") {
				st.disp[k] = 0
			}
		}
	}
	if st.Settle {
		st.enter = 1
	}
	content := Rect{388, 130, 556, 398}
	p.card(content, colCard)
	from := len(p.f.Texts)
	switch {
	case st.Settings:
		p.viewSettings(content)
	case st.Tab == 0:
		p.viewTrians(content)
	case st.Tab == 1:
		p.viewStats(content)
	case st.Tab == 2:
		p.viewHunt(content)
	case st.Tab == 3:
		p.viewLog(content)
	case st.Tab == 4:
		p.viewHistory(content)
	case st.Tab == 5:
		p.viewAuction(content)
	case st.Tab == 6:
		p.viewLuck(content)
	case st.Tab == 7:
		p.viewWishlist(content)
	}
	p.fade(content.Inset(1, 1), colCard, from, easeOut(st.enter))
	p.text(Rect{winW - 14 - 260, winH - 19, 260, 14}, "made by crysticat", 10, false, colDim, 2)
	_ = s
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.3f", v)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}
