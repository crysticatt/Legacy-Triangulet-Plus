package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Your own pulls, kept between runs (odds.json): how many capsules were opened per capsule type and how many
// of each rarity came out. Two things use it:
//   - Hunt mode blends the site's advertised Mystical odds with what you actually measured;
//   - the Luck tab compares what you pulled with what the odds say you should have pulled.

type capOdds struct {
	Pulls    int            `json:"pulls"`
	ByRarity map[string]int `json:"by"`
}

func oddsPath(dir string) string { return filepath.Join(dir, "odds.json") }

func (h *Hub) loadOdds() {
	h.oddsMu.Lock()
	defer h.oddsMu.Unlock()
	h.odds = map[string]*capOdds{}
	if b, err := os.ReadFile(oddsPath(h.primary.dir)); err == nil {
		_ = json.Unmarshal(b, &h.odds)
	}
	for k, v := range h.odds {
		if v == nil {
			delete(h.odds, k)
		} else if v.ByRarity == nil {
			v.ByRarity = map[string]int{}
		}
	}
}

func (h *Hub) recordOdds(capsule, rarity string) {
	if capsule == "" {
		return
	}
	k := strings.ToLower(capsule)
	h.oddsMu.Lock()
	c := h.odds[k]
	if c == nil {
		c = &capOdds{ByRarity: map[string]int{}}
		if h.odds == nil {
			h.odds = map[string]*capOdds{}
		}
		h.odds[k] = c
	}
	c.Pulls++
	if rarity != "" {
		c.ByRarity[rarity]++
	}
	h.oddsDirty = true
	h.oddsMu.Unlock()
}

func (h *Hub) flushOdds() {
	h.oddsMu.Lock()
	if !h.oddsDirty {
		h.oddsMu.Unlock()
		return
	}
	b, _ := json.Marshal(h.odds)
	h.oddsDirty = false
	h.oddsMu.Unlock()
	_ = os.WriteFile(oddsPath(h.primary.dir), b, 0o600)
}

// measuredMystical returns, per capsule (lower-case), {pulls, Mystical pulls}.
func (h *Hub) measuredMystical() map[string][2]int {
	h.oddsMu.Lock()
	defer h.oddsMu.Unlock()
	out := make(map[string][2]int, len(h.odds))
	for k, v := range h.odds {
		out[k] = [2]int{v.Pulls, v.ByRarity["Mystical"]}
	}
	return out
}

// huntPrior is how many capsules the advertised odds are "worth": with this many measured pulls the
// measurement and the advertised number count equally.
const huntPrior = 2000

// ---------- luck ----------

// LuckRow compares one rarity's real count with what the odds predict.
type LuckRow struct {
	Rarity   string
	Actual   int
	Expected float64
	Pct      float64 // 0..100: how many players would have done worse (luckier than Pct% of players)
}

// LuckInfo is the Luck tab's numbers.
type LuckInfo struct {
	Pulls    int // capsules counted (only capsules whose odds are known)
	AllPulls int // every capsule ever recorded
	Rows     []LuckRow
}

var luckRarities = []string{"Mystical", "Legendary", "Epic", "Rare"}

// rarityChance is the chance (0..1) that one capsule of this type gives the rarity, from the site's odds.
func rarityChance(chances map[string]float64, capsule, rarity string) (p float64, known bool) {
	for k, ch := range chances {
		i := strings.Index(k, "\x00")
		if i < 0 || !strings.EqualFold(k[:i], capsule) {
			continue
		}
		known = true
		if builtinRarity(capsule, k[i+1:]) == rarity {
			p += ch / 100
		}
	}
	return math.Min(p, 1), known
}

// poissonCDFBelow returns P(X < k) for X ~ Poisson(mu).
func poissonCDFBelow(k int, mu float64) float64 {
	if k <= 0 {
		return 0
	}
	term, sum := math.Exp(-mu), 0.0
	for i := 0; i < k; i++ {
		sum += term
		term *= mu / float64(i+1)
		if sum >= 1 {
			return 1
		}
	}
	return math.Min(sum, 1)
}

func normalCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// luckPct: how likely a typical player would have pulled fewer than `actual` (so 90 = luckier than 90%).
func luckPct(actual int, expected, variance float64) float64 {
	if expected <= 0 {
		return 50
	}
	if expected < 30 {
		return 100 * poissonCDFBelow(actual, expected)
	}
	if variance <= 0 {
		return 50
	}
	return 100 * normalCDF((float64(actual)-0.5-expected)/math.Sqrt(variance))
}

func luckFrom(odds map[string]*capOdds, chances map[string]float64) LuckInfo {
	var info LuckInfo
	type acc struct {
		actual int
		e, v   float64
	}
	byR := map[string]*acc{}
	for _, r := range luckRarities {
		byR[r] = &acc{}
	}
	for capLower, co := range odds {
		info.AllPulls += co.Pulls
		capName := titleCapsule(capLower)
		if _, known := rarityChance(chances, capName, "Mystical"); !known {
			continue
		}
		info.Pulls += co.Pulls
		for _, r := range luckRarities {
			p, _ := rarityChance(chances, capName, r)
			a := byR[r]
			a.actual += co.ByRarity[r]
			a.e += float64(co.Pulls) * p
			a.v += float64(co.Pulls) * p * (1 - p)
		}
	}
	if info.Pulls == 0 {
		return info
	}
	for _, r := range luckRarities {
		a := byR[r]
		info.Rows = append(info.Rows, LuckRow{Rarity: r, Actual: a.actual, Expected: a.e, Pct: luckPct(a.actual, a.e, a.v)})
	}
	return info
}

func (h *Hub) luck() LuckInfo {
	p := h.primary
	p.mu.Lock()
	chances := p.chances
	p.mu.Unlock()
	h.oddsMu.Lock()
	cp := make(map[string]*capOdds, len(h.odds))
	for k, v := range h.odds {
		c := &capOdds{Pulls: v.Pulls, ByRarity: map[string]int{}}
		for r, n := range v.ByRarity {
			c.ByRarity[r] = n
		}
		cp[k] = c
	}
	h.oddsMu.Unlock()
	return luckFrom(cp, chances)
}
