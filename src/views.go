package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ---------- Trians ----------

func (p *painter) filteredInv() []InvItem { return p.filterList(p.s.Inv) }

func (p *painter) filterList(list []InvItem) []InvItem {
	q := strings.ToLower(strings.TrimSpace(p.st.Search))
	if q == "" {
		return list
	}
	var out []InvItem
	for _, it := range list {
		if strings.Contains(strings.ToLower(it.Name+" "+it.Capsule+" "+it.Rarity), q) {
			out = append(out, it)
		}
	}
	return out
}

func (p *painter) trianIcon(it InvItem, r Rect) {
	if p.st.ImgFor != nil {
		if img := p.st.ImgFor(it.Name); img != nil {
			p.c.Image(img, r, 6)
			return
		}
	}
	col := rarityColors[it.Rarity]
	if it.Rarity == "" {
		col = colDim
	}
	p.c.RoundRect(r, 6, colField)
	cx, cy, s := float64(r.X)+float64(r.W)/2, float64(r.Y)+float64(r.H)/2, float64(r.W)
	p.c.Tri(pt{cx, cy - s*0.28}, pt{cx - s*0.3, cy + s*0.24}, pt{cx + s*0.3, cy + s*0.24}, col.Op(0.9))
}

func (p *painter) viewTrians(r Rect) {
	s, st := p.s, p.st
	p.label(r.X+12, r.Y+10, "YOUR TRIANS")
	pct := 0.0
	if s.TotalTrians > 0 {
		pct = float64(s.Collected) / float64(s.TotalTrians)
	}
	sum := fmt.Sprintf("%s unique  ·  %s total", fmtInt(len(s.Inv)), fmtInt(s.InvTotal))
	if s.TotalTrians > 0 && len(s.Inv) > 0 {
		sum = fmt.Sprintf("%d / %d collected (%d%%)  ·  %s total", s.Collected, s.TotalTrians, int(pct*100+0.5), fmtInt(s.InvTotal))
	}
	p.text(Rect{r.X + 12, r.Y + 28, 250, 16}, sum, 12, false, colMuted, 0)
	track := Rect{r.X + 12, r.Y + 47, 250, 4}
	p.c.RoundRect(track, 2, colField)
	if w := int(p.st.num("sg:coll", pct) * float64(track.W)); w > 0 {
		p.c.RoundRect(Rect{track.X, track.Y, maxi(w, 4), 4}, 2, colGood)
	}
	sf := Rect{r.X + r.W - 12 - 170, r.Y + 8, 170, 28}
	p.seg("tv", Rect{sf.X - 8 - 130, r.Y + 8, 130, 28}, []string{"Owned", "Missing"}, st.TriansView)
	p.field(sf, "search")
	p.f.Edits["search"] = Rect{sf.X + 9, sf.Y + 6, sf.W - 18, 16}
	if st.Search == "" && st.Focus != "search" {
		p.text(Rect{sf.X + 9, sf.Y, sf.W - 18, sf.H}, "search trians", 12, false, colDim, 0)
	}

	area := Rect{r.X + 12, r.Y + 54, r.W - 24, r.H - 64}
	items := p.filteredInv()
	if st.TriansView == 1 {
		items = p.filterList(s.Missing)
	}
	if len(items) == 0 {
		msg := "No trians yet. Log in and press refresh."
		if st.TriansView == 1 && len(s.Inv) > 0 && len(s.Missing) == 0 {
			msg = "You own every trian in the game."
		} else if len(s.Inv) > 0 || st.TriansView == 1 {
			msg = "Nothing matches that search."
		}
		p.text(area, msg, 13, false, colDim, 1)
		return
	}
	cw, ch := (area.W-8)/2, 38
	rows := (len(items) + 1) / 2
	total := rows * (ch + 4)
	maxScroll := maxi(0, total-area.H)
	p.f.MaxScroll["inv"] = maxScroll
	off := mini(maxi(st.Scroll["inv"], 0), maxScroll)
	p.c.SetClip(area)
	for i, it := range items {
		rx, ry := i%2, i/2
		cell := Rect{area.X + rx*(cw+8), area.Y + ry*(ch+4) - off, cw, ch}
		if cell.Y+cell.H < area.Y || cell.Y > area.Y+area.H {
			// start loading pictures a few screens ahead so they are ready when you scroll
			if it.Qty > 0 && st.ImgFor != nil && cell.Y > area.Y-area.H && cell.Y < area.Y+2*area.H {
				st.ImgFor(it.Name)
			}
			continue
		}
		id := "inv:" + it.Name
		h := st.hv(id)
		from := len(p.f.Texts)
		p.c.RoundRect(cell, 8, lerpCol(colField, colCardHi, h))
		rc := rarityColors[it.Rarity]
		if it.Rarity == "" {
			rc = colDim
		}
		if it.Qty == 0 {
			rc = rc.Op(0.45)
		}
		p.c.RoundRect(Rect{cell.X, cell.Y + 8, 3, ch - 16}, 1.5, rc)
		if it.Qty == 0 {
			p.c.RoundRect(Rect{cell.X + 10, cell.Y + 5, 28, 28}, 6, colField)
			p.text(Rect{cell.X + 10, cell.Y + 5, 28, 28}, "?", 14, true, colDim, 1)
		} else {
			p.trianIcon(it, Rect{cell.X + 10, cell.Y + 5, 28, 28})
		}
		p.text(Rect{cell.X + 46, cell.Y + 3, cw - 46 - 56, 18}, it.Name, 13, true, colText, 0)
		sub := it.Capsule
		if it.Rarity != "" {
			sub = it.Rarity + "  ·  " + it.Capsule
		}
		p.text(Rect{cell.X + 46, cell.Y + 20, cw - 46 - 56, 15}, sub, 11, false, rc, 0)
		if it.Qty == 0 {
			p.text(Rect{cell.X + cw - 70, cell.Y, 62, ch}, "missing", 11, false, colDim, 2)
		} else {
			p.text(Rect{cell.X + cw - 58, cell.Y, 50, ch}, "x"+fmtInt(it.Qty), 14, true, colText, 2)
		}
		vis := cell.Intersect(area)
		if !vis.Empty() {
			p.f.hit(vis, id)
		}
		a := easeOut(clampf(st.enter*1.9-float64(i)*0.05, 0, 1))
		p.fade(cell, colCard, from, a)
	}
	p.c.ResetClip()
	p.f.hit(area, "scroll:inv")
	p.scrollbar(area, total, off)
}

func (p *painter) scrollbar(area Rect, total, off int) {
	if total <= area.H {
		return
	}
	th := maxi(24, area.H*area.H/total)
	ty := area.Y + (area.H-th)*off/maxi(1, total-area.H)
	p.c.RoundRect(Rect{area.X + area.W + 3, ty, 3, th}, 1.5, colAccent.Op(0.6))
}

// ---------- Stats ----------

func fmtDur(sec int) string {
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%dm %ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%dh %dm", sec/3600, sec%3600/60)
}

func (p *painter) miniStat(x, y, w int, label string, v float64, text string) {
	p.text(Rect{x, y, w, 14}, label, 10, true, colMuted, 0)
	if text == "" {
		text = fmtInt(int(p.st.num("m:"+label, v) + 0.5))
	}
	p.text(Rect{x, y + 14, w, 24}, text, 20, true, colText, 0)
}

func (p *painter) viewStats(r Rect) {
	s, st := p.s, p.st
	p.label(r.X+12, r.Y+10, "THIS SESSION")
	w := (r.W - 24) / 4
	x := r.X + 12
	p.miniStat(x, r.Y+30, w, "OPENED", float64(s.Opened), "")
	p.miniStat(x+w, r.Y+30, w, "PER HOUR", s.PerMin*60, "")
	p.miniStat(x+2*w, r.Y+30, w, "WORKERS", float64(s.WorkersRunning), "")
	up := "-"
	if s.StartedAgo > 0 {
		up = fmtDur(s.StartedAgo)
	}
	p.miniStat(x+3*w, r.Y+30, w, "RUNNING FOR", 0, up)

	// graph: pulls per minute, from the sampled totals
	g := Rect{r.X + 12, r.Y + 92, r.W - 24, 138}
	p.c.RoundRect(g, 8, colField)
	p.label(g.X+10, g.Y+7, "PULLS PER MINUTE - LAST 30 MIN")
	plot := Rect{g.X + 10, g.Y + 28, g.W - 20, g.H - 38}
	for i := 0; i < 4; i++ {
		y := float64(plot.Y) + float64(i)*float64(plot.H)/3
		p.c.Line(float64(plot.X), y, float64(plot.X+plot.W), y, 1, colEdge.Op(0.5))
	}
	rates := ratesFrom(s.Series, sampleEvery.Seconds())
	if len(rates) < 2 {
		p.text(plot, "Start opening to see the graph", 12, false, colDim, 1)
	} else {
		maxv := 1.0
		for _, v := range rates {
			maxv = math.Max(maxv, v)
		}
		maxv *= 1.15
		reveal := st.num("sg:graph", 1)
		pts := make([]pt, len(rates))
		for i, v := range rates {
			pts[i] = pt{float64(plot.X) + float64(i)*float64(plot.W)/float64(len(rates)-1), float64(plot.Y+plot.H) - v/maxv*float64(plot.H)}
		}
		limit := float64(plot.X) + reveal*float64(plot.W)
		p.c.SetClip(Rect{plot.X - 2, plot.Y - 4, int(limit) - plot.X + 4, plot.H + 8}.Intersect(Rect{g.X, g.Y, g.W, g.H}))
		p.c.AreaUnder(pts, float64(plot.Y+plot.H), colAccent, 0.35)
		for i := 0; i+1 < len(pts); i++ {
			p.c.Line(pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], 2, colCyan)
		}
		p.c.ResetClip()
		if reveal > 0.98 {
			e := pts[len(pts)-1]
			ph := math.Mod(st.Now, 1.6) / 1.6
			if s.Running {
				p.c.Circle(e[0], e[1], 4+8*ph, colCyan.Op(0.4*(1-ph)))
			}
			p.c.Circle(e[0], e[1], 3.5, colCyan)
		}
		p.text(Rect{g.X + g.W - 110, g.Y + 6, 100, 16}, fmt.Sprintf("peak %s/min", fmtInt(int(maxv/1.15))), 11, false, colMuted, 2)
	}

	// rarity bars
	p.label(r.X+12, r.Y+240, "PULLS BY RARITY")
	total := 0
	maxc := 1
	for _, n := range rarityOrder {
		total += s.Counts[n]
		maxc = maxi(maxc, s.Counts[n])
	}
	by := r.Y + 260
	for i, name := range rarityOrder {
		y := by + i*19
		col := rarityColors[name]
		n := s.Counts[name]
		dim := n == 0
		dc := col
		if dim {
			dc = col.Op(0.3)
		}
		p.c.Circle(float64(r.X+18), float64(y+9), 4, dc)
		p.text(Rect{r.X + 30, y, 84, 18}, name, 12, false, map[bool]Col{true: colDim, false: colMuted}[dim], 0)
		track := Rect{r.X + 120, y + 5, r.W - 120 - 90, 9}
		p.c.RoundRect(track, 4.5, colField)
		frac := st.num("bar:"+name, float64(n)/float64(maxc))
		if bw := int(frac * float64(track.W)); bw > 0 {
			p.c.RoundRect(Rect{track.X, track.Y, maxi(bw, 9), track.H}, 4.5, col)
		}
		pct := ""
		if total > 0 {
			pct = fmt.Sprintf("%.1f%%", float64(n)*100/float64(total))
		}
		p.text(Rect{r.X + r.W - 84, y, 36, 18}, pct, 11, false, colDim, 2)
		p.text(Rect{r.X + r.W - 52, y, 40, 18}, fmtInt(n), 12, true, map[bool]Col{true: colDim, false: colText}[dim], 2)
	}
}

// ratesFrom turns cumulative totals (one per `every` seconds) into pulls per minute,
// smoothed over about half a minute, for up to the last 30 minutes.
func ratesFrom(series []int, every float64) []float64 {
	if every <= 0 {
		every = 1
	}
	if len(series) > 361 {
		series = series[len(series)-361:]
	}
	win := 6
	var out []float64
	for i := 1; i < len(series); i++ {
		j := maxi(0, i-win)
		span := float64(i-j) * every
		out = append(out, float64(series[i]-series[j])/span*60)
	}
	return out
}

// ---------- Hunt ----------

func (p *painter) viewHunt(r Rect) {
	s, st := p.s, p.st
	p.label(r.X+12, r.Y+10, "MYSTICAL HUNT")
	msg := "Set Mode to Hunt on the left to aim the workers at these."
	if s.Mode == "hunt" {
		msg = fmt.Sprintf("Hunting with %d worker(s).", s.Workers)
		if len(s.Hunt) > 0 && s.Hunt[0].Chasing {
			msg += " Ranked by odds of a Mystical you don't own."
		} else {
			msg += " Ranked by Mystical odds."
		}
	}
	p.text(Rect{r.X + 12, r.Y + 28, r.W - 24 - 190, 16}, msg, 11, false, colMuted, 0)
	goal := "off"
	if s.StopAfter > 0 {
		goal = fmt.Sprintf("%d", s.StopAfter)
	}
	p.text(Rect{r.X + r.W - 12 - 100 - 112, r.Y + 6, 106, 24}, "Stop after Mystical", 11, false, colMuted, 2)
	p.stepper("goal", Rect{r.X + r.W - 12 - 100, r.Y + 6, 100, 24}, goal)
	p.toggle("chase", Rect{r.X + r.W - 12 - 190, r.Y + 32, 190, 20}, "Chase ones I don't own", s.HuntMissing)

	rows := s.Hunt
	if len(rows) == 0 {
		rows = huntRanking(nil, nil, nil)
	}
	best := 0.0
	for _, h := range rows {
		if h.Chasing {
			best = math.Max(best, h.NewOdds)
		} else {
			best = math.Max(best, h.Odds)
		}
	}
	top := r.Y + 58
	for i, h := range rows {
		if i >= 9 {
			break
		}
		y := top + i*26
		from := len(p.f.Texts)
		row := Rect{r.X + 12, y, r.W - 24, 24}
		hot := h.Workers > 0
		p.c.RoundRect(row, 7, lerpCol(colField, colCardHi, b2f(hot)*0.8))
		p.text(Rect{row.X + 8, y, 22, 24}, fmt.Sprintf("%d", i+1), 12, true, colDim, 0)
		p.text(Rect{row.X + 34, y, 84, 24}, h.Capsule, 13, true, colText, 0)
		track := Rect{row.X + 124, y + 8, 200, 8}
		p.c.RoundRect(track, 4, colBg)
		frac := 0.0
		if best > 0 {
			frac = h.Odds / best
			if h.Chasing {
				frac = h.NewOdds / best
			}
		}
		frac = st.num("bar:h"+h.Capsule, frac)
		if bw := int(frac * float64(track.W)); bw > 0 {
			p.c.RoundRect(Rect{track.X, track.Y, maxi(bw, 8), 8}, 4, rarityColors["Mystical"])
		}
		odds := fmt.Sprintf("%s%%", trimFloat(h.Odds))
		if h.Chasing {
			odds = fmt.Sprintf("%s%% new", trimFloat(h.NewOdds))
		}
		if h.Known && h.Measured > 0 && h.Pulls >= 200 {
			odds += "*" // blended with the odds measured from your own pulls
		}
		if !h.Known {
			odds = fmt.Sprintf("%d Mystical", h.Mystics)
		}
		p.text(Rect{row.X + 332, y, 84, 24}, odds, 12, true, colText, 2)
		if h.Workers > 0 {
			badge := Rect{row.X + row.W - 84, y + 4, 76, 16}
			p.c.RoundRect(badge, 8, colAccent.Op(0.25))
			p.text(badge, fmt.Sprintf("%d worker%s", h.Workers, map[bool]string{true: "", false: "s"}[h.Workers == 1]), 10, true, colCyan, 1)
		}
		p.fade(row, colCard, from, easeOut(clampf(st.enter*1.9-float64(i)*0.06, 0, 1)))
	}

	// the numbers
	sy := r.Y + r.H - 78
	p.c.Line(float64(r.X+12), float64(sy-8), float64(r.X+r.W-12), float64(sy-8), 1, colEdge.Op(0.7))
	w := (r.W - 24) / 4
	x := r.X + 12
	packs := "-"
	if s.HasAcct {
		packs = ""
	}
	p.miniStat(x, sy, w, "PACKS LEFT", float64(s.PacksLeft), packs)
	p.miniStat(x+w, sy, w, "SINCE LAST", float64(s.SinceMystical), "")
	oneIn, coinsPer := "-", "-"
	if s.PacksPerMystical > 0 {
		oneIn = "1 in " + fmtInt(int(s.PacksPerMystical+0.5))
		coinsPer = fmtInt(int(s.CoinsPerMystical + 0.5))
	}
	p.miniStat(x+2*w, sy, w, "MEASURED", 0, oneIn)
	p.miniStat(x+3*w, sy, w, "COINS / MYSTICAL", 0, coinsPer)
	p.text(Rect{x, sy + 42, r.W - 24, 16}, fmt.Sprintf("%d coins per capsule. Measured numbers sharpen as you open more.", s.CoinCost), 11, false, colDim, 0)
}

// ---------- Auction ----------

func (p *painter) viewAuction(r Rect) {
	s, st := p.s, p.st
	p.label(r.X+12, r.Y+10, "SELL ON THE AUCTION")
	list := Rect{r.X + 12, r.Y + 34, 250, r.H - 46}
	p.c.RoundRect(list, 8, colField)
	if len(s.Inv) == 0 {
		p.text(list, "No trians loaded yet.", 12, false, colDim, 1)
	} else {
		rowH := 30
		total := len(s.Inv) * rowH
		maxScroll := maxi(0, total-list.H+8)
		p.f.MaxScroll["auc"] = maxScroll
		off := mini(maxi(st.Scroll["auc"], 0), maxScroll)
		inner := list.Inset(4, 4)
		p.f.hit(list, "scroll:auc") // first, so the rows registered after it stay on top
		p.c.SetClip(inner)
		for i, it := range s.Inv {
			row := Rect{inner.X, inner.Y + i*rowH - off, inner.W, rowH - 2}
			if row.Y+row.H < inner.Y || row.Y > inner.Y+inner.H {
				continue
			}
			id := "pick:" + it.Name
			sel := st.Sel == it.Name
			h := st.hv(id)
			sv := st.num("sg:pick:"+it.Name, b2f(sel))
			p.c.RoundRect(row, 6, lerpCol(lerpCol(colField, colCardHi, h), colAccentD, sv))
			rc := rarityColors[it.Rarity]
			if it.Rarity == "" {
				rc = colDim
			}
			p.c.Circle(float64(row.X+10), float64(row.Y+row.H/2), 3.5, rc)
			p.text(Rect{row.X + 20, row.Y, row.W - 70, row.H}, it.Name, 12, sel, colText, 0)
			p.text(Rect{row.X + row.W - 50, row.Y, 44, row.H}, "x"+fmtInt(it.Qty), 12, true, colMuted, 2)
			if vis := row.Intersect(inner); !vis.Empty() {
				p.f.hit(vis, id)
			}
		}
		p.c.ResetClip()
		p.scrollbar(inner, total, off)
	}

	fx := r.X + 278
	fw := r.W - 278 - 12
	if st.Sel == "" {
		p.text(Rect{fx, r.Y + 34, fw, 40}, "Pick a trian from the list.", 13, false, colMuted, 0)
	} else {
		qty := 0
		for _, it := range s.Inv {
			if it.Name == st.Sel {
				qty = it.Qty
			}
		}
		p.text(Rect{fx, r.Y + 34, fw, 22}, st.Sel, 16, true, colText, 0)
		p.text(Rect{fx, r.Y + 56, fw, 16}, fmt.Sprintf("you own x%s", fmtInt(qty)), 12, false, colMuted, 0)
	}
	labels := []struct{ key, text string }{{"aamount", "Amount of trians"}, {"abid", "Starting bid"}, {"apay", "Payout (0 = none)"}}
	for i, l := range labels {
		y := r.Y + 84 + i*56
		p.text(Rect{fx, y, fw, 14}, l.text, 11, false, colMuted, 0)
		fr := Rect{fx, y + 17, fw, 28}
		p.field(fr, l.key)
		p.f.Edits[l.key] = Rect{fr.X + 9, fr.Y + 6, fr.W - 18, 16}
	}
	by := r.Y + 84 + 3*56
	if s.AuctionBusy {
		p.button("auction", Rect{fx, by, fw, 34}, "Listing...", colDim, 0, 13)
	} else {
		p.button("auction", Rect{fx, by, fw, 34}, "List on auction", colAccent, 0, 13)
	}
	if s.AuctionMsg != "" {
		col := colGood
		if strings.HasPrefix(s.AuctionMsg, "Failed") {
			col = colBad
		}
		p.text(Rect{fx, by + 38, fw, 30}, s.AuctionMsg, 11, false, col, 0)
	}
}

// ---------- Log ----------

func (p *painter) viewLog(r Rect) {
	s, st := p.s, p.st
	p.label(r.X+12, r.Y+10, "PULL LOG")
	p.text(Rect{r.X + 12, r.Y + 28, 260, 16}, "Epic or better, and every new trian", 12, false, colMuted, 0)
	p.button("openlog", Rect{r.X + r.W - 12 - 96, r.Y + 8, 96, 26}, "Open CSV", colCard, 1, 12)
	p.toggle("sound", Rect{r.X + r.W - 12 - 96 - 8 - 150, r.Y + 8, 150, 26}, "Sound alerts", s.AlertSound)

	area := Rect{r.X + 12, r.Y + 54, r.W - 24, r.H - 64}
	if len(s.Log) == 0 {
		p.text(area, "Nothing yet. Rare pulls and new trians will appear here.", 13, false, colDim, 1)
		return
	}
	rowH := 30
	total := len(s.Log) * rowH
	maxScroll := maxi(0, total-area.H)
	p.f.MaxScroll["log"] = maxScroll
	off := mini(maxi(st.Scroll["log"], 0), maxScroll)
	p.c.SetClip(area)
	for i, ev := range s.Log {
		row := Rect{area.X, area.Y + i*rowH - off, area.W - 8, rowH - 3}
		if row.Y+row.H < area.Y || row.Y > area.Y+area.H {
			continue
		}
		from := len(p.f.Texts)
		id := "log:" + fmt.Sprint(i)
		p.c.RoundRect(row, 7, lerpCol(colField, colCardHi, st.hv(id)))
		rc := rarityColors[ev.Rarity]
		p.c.RoundRect(Rect{row.X, row.Y + 6, 3, row.H - 12}, 1.5, rc)
		p.text(Rect{row.X + 12, row.Y, 64, row.H}, ev.Time.Format("15:04:05"), 11, false, colDim, 0)
		p.text(Rect{row.X + 82, row.Y, 74, row.H}, ev.Rarity, 12, true, rc, 0)
		p.text(Rect{row.X + 160, row.Y, 170, row.H}, ev.Trian, 13, true, colText, 0)
		p.text(Rect{row.X + 336, row.Y, 100, row.H}, ev.Capsule, 12, false, colMuted, 0)
		if ev.New {
			b := Rect{row.X + row.W - 54, row.Y + 6, 46, row.H - 12}
			p.c.RoundRect(b, 7, colGood.Op(0.22))
			p.text(b, "NEW", 10, true, colGood, 1)
		}
		if vis := row.Intersect(area); !vis.Empty() {
			p.f.hit(vis, id)
		}
		p.fade(row, colCard, from, easeOut(clampf(st.enter*1.9-float64(i)*0.04, 0, 1)))
	}
	p.c.ResetClip()
	p.f.hit(area, "scroll:log")
	p.scrollbar(area, total, off)
}

// viewHistory draws trends over days: pulls per hour/day, Mystical and Chroma counts, and coins.
func (p *painter) viewHistory(r Rect) {
	s, st := p.s, p.st
	now := st.Wall
	if now.IsZero() {
		now = time.Now()
	}
	rng := mini(maxi(st.HistRange, 0), 2)
	pts := histSeries(s.Hist, now, rng)
	p.label(r.X+12, r.Y+14, "HISTORY")
	p.seg("hr", Rect{r.X + r.W - 12 - 210, r.Y + 8, 210, 28}, histRangeNames, rng)

	total, myst, chroma, best := 0, 0, 0, 0
	maxv := 1
	for _, pt := range pts {
		total += pt.Pulls
		myst += pt.Mystical
		chroma += pt.Chroma
		maxv = maxi(maxv, pt.Pulls)
		best = maxi(best, pt.Pulls)
	}
	unit := "hour"
	if rng > 0 {
		unit = "day"
	}
	w := (r.W - 24) / 4
	x := r.X + 12
	avg := float64(total) / float64(len(pts))
	p.miniStat(x, r.Y+44, w, "OPENED", float64(total), "")
	p.miniStat(x+w, r.Y+44, w, "PER "+strings.ToUpper(unit), avg, "")
	p.miniStat(x+2*w, r.Y+44, w, "MYSTICAL", float64(myst), "")
	p.miniStat(x+3*w, r.Y+44, w, "CHROMA", float64(chroma), "")

	// bars: capsules opened per hour/day; a pink dot marks a Mystical, a cyan one a Chroma
	g := Rect{r.X + 12, r.Y + 92, r.W - 24, 150}
	p.c.RoundRect(g, 8, colField)
	p.label(g.X+10, g.Y+7, "CAPSULES PER "+strings.ToUpper(unit))
	if myst > 0 {
		p.text(Rect{g.X + g.W - 190, g.Y + 6, 180, 16}, "1 Mystical per "+fmtInt(total/myst)+" capsules", 11, false, colMuted, 2)
	}
	plot := Rect{g.X + 10, g.Y + 40, g.W - 20, g.H - 62}
	for i := 0; i < 3; i++ {
		y := float64(plot.Y) + float64(i)*float64(plot.H)/2
		p.c.Line(float64(plot.X), y, float64(plot.X+plot.W), y, 1, colEdge.Op(0.5))
	}
	if total == 0 {
		p.text(plot, "Nothing opened in this time. Start opening to build history.", 12, false, colDim, 1)
	} else {
		reveal := st.num("sg:graph", 1)
		slotW := float64(plot.W) / float64(len(pts))
		bw := math.Max(2, slotW-math.Max(2, slotW*0.25))
		for i, pt := range pts {
			bx := float64(plot.X) + float64(i)*slotW + (slotW-bw)/2
			hh := float64(pt.Pulls) / float64(maxv) * float64(plot.H) * math.Min(1, reveal)
			if pt.Pulls > 0 {
				hh = math.Max(hh, 2)
				p.c.RoundRect(Rect{int(bx), plot.Y + plot.H - int(hh), int(bw), int(hh)}, math.Min(3, bw/2), colAccent)
			}
			cx := bx + bw/2
			if pt.Mystical > 0 {
				p.c.Circle(cx, float64(plot.Y-6), 3.5, rarityColors["Mystical"])
			} else if pt.Chroma > 0 {
				p.c.Circle(cx, float64(plot.Y-6), 3.5, rarityColors["Chroma"])
			}
		}
		p.text(Rect{plot.X, g.Y + g.H - 18, 80, 14}, pts[0].Label, 10, false, colDim, 0)
		p.text(Rect{plot.X + plot.W - 80, g.Y + g.H - 18, 80, 14}, pts[len(pts)-1].Label, 10, false, colDim, 2)
		p.text(Rect{plot.X + plot.W/2 - 60, g.Y + g.H - 18, 120, 14}, "peak "+fmtInt(best)+"/"+unit, 10, false, colDim, 1)
	}

	// coins line
	c2 := Rect{r.X + 12, r.Y + 250, r.W - 24, 138}
	p.c.RoundRect(c2, 8, colField)
	p.label(c2.X+10, c2.Y+7, "COINS")
	cp := Rect{c2.X + 10, c2.Y + 30, c2.W - 20, c2.H - 52}
	minC, maxC, known := math.MaxFloat64, 0.0, 0
	for _, pt := range pts {
		if pt.Coins >= 0 {
			known++
			minC = math.Min(minC, float64(pt.Coins))
			maxC = math.Max(maxC, float64(pt.Coins))
		}
	}
	if known < 2 {
		p.text(cp, "Coins appear here once the opener has run for a while", 12, false, colDim, 1)
		return
	}
	span := math.Max(maxC-minC, 1)
	lo, hi := minC-span*0.1, maxC+span*0.1
	var line []pt
	for i, q := range pts {
		if q.Coins < 0 {
			continue
		}
		line = append(line, pt{float64(cp.X) + float64(i)*float64(cp.W)/float64(len(pts)-1), float64(cp.Y+cp.H) - (float64(q.Coins)-lo)/(hi-lo)*float64(cp.H)})
	}
	p.c.AreaUnder(line, float64(cp.Y+cp.H), colGood, 0.3)
	for i := 0; i+1 < len(line); i++ {
		p.c.Line(line[i][0], line[i][1], line[i+1][0], line[i+1][1], 2, colGood)
	}
	last := pts[len(pts)-1].Coins
	p.text(Rect{c2.X + c2.W - 290, c2.Y + 6, 280, 16}, fmt.Sprintf("now %s  ·  high %s  ·  low %s", fmtInt(last), fmtInt(int(maxC)), fmtInt(int(minC))), 11, false, colMuted, 2)
	p.text(Rect{cp.X, c2.Y + c2.H - 18, 80, 14}, pts[0].Label, 10, false, colDim, 0)
	p.text(Rect{cp.X + cp.W - 80, c2.Y + c2.H - 18, 80, 14}, pts[len(pts)-1].Label, 10, false, colDim, 2)
}

// ---------- Luck ----------

func etaText(packs, rate float64) string {
	if packs <= 0 {
		return "done"
	}
	t := fmtInt(int(packs+0.5)) + " packs"
	if rate > 0 {
		t += " (~" + fmtDur(int(packs/rate)) + ")"
	}
	return t
}

func (p *painter) viewLuck(r Rect) {
	s := p.s
	p.label(r.X+12, r.Y+10, "FIRST COLLECTION ETA")
	p.button("diag", Rect{r.X + r.W - 12 - 130, r.Y + 6, 130, 26}, diagCaption(p.st), colCard, 1, 12)
	y := r.Y + 32
	if len(s.ETA.Groups) == 0 {
		p.text(Rect{r.X + 12, y, r.W - 24, 16}, "Log in and load your trians to see how long the rest will take.", 12, false, colDim, 0)
		y += 20
	}
	for _, g := range s.ETA.Groups {
		if y > r.Y+r.H-150 {
			break
		}
		p.text(Rect{r.X + 12, y, 150, 18}, g.Name, 12, true, colText, 0)
		txt := fmt.Sprintf("%d missing: %s", g.Missing, etaText(g.Packs, s.ETA.Rate))
		p.text(Rect{r.X + 150, y, r.W - 162, 18}, txt, 12, false, colMuted, 0)
		y += 20
	}
	y = r.Y + r.H - 140
	p.label(r.X+12, y, "LUCK (vs the site's odds)")
	y += 22
	if s.Luck.Pulls == 0 {
		p.text(Rect{r.X + 12, y, r.W - 24, 16}, "Open some packs and your luck shows up here.", 12, false, colDim, 0)
		return
	}
	p.text(Rect{r.X + 12, y, r.W - 24, 16}, fmt.Sprintf("%s capsules counted", fmtInt(s.Luck.Pulls)), 11, false, colDim, 0)
	y += 18
	for _, l := range s.Luck.Rows {
		p.text(Rect{r.X + 12, y, 90, 18}, l.Rarity, 12, true, colText, 0)
		p.text(Rect{r.X + 104, y, r.W - 116, 18}, fmt.Sprintf("got %d, expected %.1f: luckier than %.0f%% of players", l.Actual, l.Expected, l.Pct), 12, false, colMuted, 0)
		y += 20
	}
}

func diagCaption(st *UIState) string {
	if st.DiagAt > 0 && st.Now-st.DiagAt < 2.5 {
		return "Copied!"
	}
	return "Copy diagnostics"
}

// ---------- Wishlist ----------

func (p *painter) viewWishlist(r Rect) {
	s, st := p.s, p.st
	wished := map[string]bool{}
	for _, w := range s.Wishlist {
		wished[lowerKey(w)] = true
	}
	qty := map[string]int{}
	for _, it := range s.Inv {
		qty[lowerKey(it.Name)] += it.Qty
	}
	p.label(r.X+12, r.Y+10, "WISHLIST")
	p.text(Rect{r.X + 12, r.Y + 28, r.W - 24, 16}, fmt.Sprintf("%d starred. Click a trian to star it. Use the Wish mode to go for them; you get an alert on the first drop.", len(s.Wishlist)), 11, false, colMuted, 0)
	// starred first, then the rest in game order
	var list []builtinTrian
	for _, t := range builtinTrians {
		if wished[lowerKey(t.Name)] {
			list = append(list, t)
		}
	}
	for _, t := range builtinTrians {
		if !wished[lowerKey(t.Name)] {
			list = append(list, t)
		}
	}
	area := Rect{r.X + 12, r.Y + 52, r.W - 24, r.H - 62}
	rowH := 28
	total := len(list) * rowH
	maxScroll := maxi(0, total-area.H+8)
	p.f.MaxScroll["wish"] = maxScroll
	off := mini(maxi(st.Scroll["wish"], 0), maxScroll)
	p.f.hit(area, "scroll:wish") // first, so the rows registered after it stay on top
	p.c.SetClip(area)
	for i, t := range list {
		row := Rect{area.X, area.Y + i*rowH - off, area.W - 10, rowH - 2}
		if row.Y+row.H < area.Y || row.Y > area.Y+area.H {
			continue
		}
		id := "wish:" + t.Name
		on := wished[lowerKey(t.Name)]
		h := st.hv(id)
		fill := lerpCol(colField, colCardHi, h)
		if on {
			fill = lerpCol(colAccentD, colAccent, h*0.4)
		}
		p.c.RoundRect(row, 6, fill)
		star := "☆"
		sc := colDim
		if on {
			star, sc = "★", colWarn
		}
		p.text(Rect{row.X + 6, row.Y, 22, row.H}, star, 14, true, sc, 1)
		rc := rarityColors[t.Rarity]
		p.c.Circle(float64(row.X+40), float64(row.Y+row.H/2), 3.5, rc)
		p.text(Rect{row.X + 52, row.Y, row.W - 200, row.H}, t.Name, 12, on, colText, 0)
		p.text(Rect{row.X + row.W - 150, row.Y, 70, row.H}, t.Capsule, 11, false, colDim, 0)
		q := qty[lowerKey(t.Name)]
		qs, qc := "x"+fmtInt(q), colMuted
		if q == 0 {
			qs, qc = "missing", colBad
		}
		p.text(Rect{row.X + row.W - 76, row.Y, 70, row.H}, qs, 12, true, qc, 2)
		if vis := row.Intersect(area); !vis.Empty() {
			p.f.hit(vis, id)
		}
	}
	p.c.ResetClip()
	p.scrollbar(area, total, off)
}
