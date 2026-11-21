package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Collection ETA: how many capsules it takes, on average, to pull every trian you are still missing.
// Each missing trian i drops with chance p_i per capsule. For one capsule type the expected number of capsules
// to collect them all is  E = integral over t of (1 - product(1 - exp(-p_i t))) dt  (the coupon collector with
// unequal odds). Capsule types are opened one after another, so the totals add up.

// ETAGroup is one goal (for example "missing Mysticals").
type ETAGroup struct {
	Name     string
	Missing  int     // trians still missing that can be pulled
	NoOdds   int     // missing trians whose drop chance is not known (left out)
	Packs    float64 // expected capsules still to open (0 = nothing missing)
	Hardest  string  // the missing trian that will take longest
	HardestP float64 // its chance per capsule, in percent
}

// ETAInfo is the Luck tab's collection numbers.
type ETAInfo struct {
	Groups   []ETAGroup
	Rate     float64 // capsules per second used for the time estimate (0 = unknown)
	CoinCost int
	Coins    int
}

// expectedPacks returns the expected capsules to collect every item with the given per-capsule chances (0..1).
func expectedPacks(ps []float64) float64 {
	var live []float64
	pmax, pmin := 0.0, math.Inf(1)
	for _, p := range ps {
		if p > 0 {
			live = append(live, p)
			pmax = math.Max(pmax, p)
			pmin = math.Min(pmin, p)
		}
	}
	switch len(live) {
	case 0:
		return 0
	case 1:
		return 1 / live[0]
	}
	// integrate over log time: t = e^u, from far before the first chance matters to far after the last
	tlo, thi := 1e-3/pmax, 60/pmin
	ulo, uhi := math.Log(tlo), math.Log(thi)
	const steps = 2400 // even
	h := (uhi - ulo) / steps
	f := func(u float64) float64 {
		t := math.Exp(u)
		prod := 1.0
		for _, p := range live {
			prod *= 1 - math.Exp(-p*t)
		}
		return (1 - prod) * t
	}
	sum := f(ulo) + f(uhi)
	for i := 1; i < steps; i++ {
		w := 4.0
		if i%2 == 0 {
			w = 2
		}
		sum += w * f(ulo+float64(i)*h)
	}
	return tlo + sum*h/3
}

// etaFrom builds the goals from the missing trians and the site's drop chances.
func etaFrom(missing []InvItem, chances map[string]float64) []ETAGroup {
	lc := make(map[string]float64, len(chances))
	for k, v := range chances {
		if i := strings.Index(k, "\x00"); i >= 0 {
			lc[strings.ToLower(k[:i])+"|"+strings.ToLower(k[i+1:])] = v
		}
	}
	goals := []struct {
		name string
		keep func(InvItem) bool
	}{
		{"Missing Mysticals", func(it InvItem) bool { return it.Rarity == "Mystical" }},
		{"Whole collection", func(it InvItem) bool { return true }},
	}
	var out []ETAGroup
	for _, g := range goals {
		grp := ETAGroup{Name: g.name}
		byCap := map[string][]float64{}
		for _, it := range missing {
			if !g.keep(it) {
				continue
			}
			ch, ok := lc[strings.ToLower(it.Capsule)+"|"+strings.ToLower(it.Name)]
			if !ok || ch <= 0 {
				grp.NoOdds++
				continue
			}
			grp.Missing++
			byCap[strings.ToLower(it.Capsule)] = append(byCap[strings.ToLower(it.Capsule)], ch/100)
			if grp.HardestP == 0 || ch < grp.HardestP {
				grp.Hardest, grp.HardestP = it.Name, ch
			}
		}
		for _, ps := range byCap {
			grp.Packs += expectedPacks(ps)
		}
		out = append(out, grp)
	}
	return out
}

// luckAndETA fills the Luck tab's numbers. Both are cached: the luck for a couple of seconds, the ETA until the
// set of missing trians (or the site's odds) changes.
func (h *Hub) luckAndETA(s Snapshot, coinCost int) (LuckInfo, ETAInfo) {
	h.mu.Lock()
	if time.Since(h.luckAt) > 2*time.Second {
		h.mu.Unlock()
		l := h.luck()
		h.mu.Lock()
		h.luckCache, h.luckAt = l, time.Now()
	}
	luck := h.luckCache
	h.mu.Unlock()

	p := h.primary
	p.mu.Lock()
	chances := p.chances
	p.mu.Unlock()
	key := fmt.Sprintf("%d/%d/%d", len(s.Missing), s.Collected, len(chances))
	h.mu.Lock()
	groups := h.etaCache
	if key != h.etaKey {
		h.mu.Unlock()
		groups = etaFrom(s.Missing, chances)
		h.mu.Lock()
		h.etaKey, h.etaCache = key, groups
	}
	h.mu.Unlock()
	if len(s.Missing) == 0 && s.Collected == 0 { // inventory not loaded yet
		groups = nil
	}
	return luck, ETAInfo{Groups: groups, Rate: h.etaRate(s), CoinCost: coinCost, Coins: s.Coins}
}

// etaRate is capsules per second: the live speed while opening, otherwise the average over the hours in the
// last week in which something was opened.
func (h *Hub) etaRate(s Snapshot) float64 {
	if s.Running && s.PerMin > 0 {
		return s.PerMin / 60
	}
	cut := time.Now().AddDate(0, 0, -7).Unix()
	h.histMu.Lock()
	defer h.histMu.Unlock()
	pulls, hours := 0, 0
	for _, b := range h.hist {
		if b.T >= cut && b.Pulls > 0 {
			pulls += b.Pulls
			hours++
		}
	}
	if hours == 0 {
		return 0
	}
	return float64(pulls) / float64(hours*3600)
}
