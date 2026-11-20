package main

import (
	"sort"
	"strings"
)

// Mystical hunting: pick the capsules where a Mystical is most likely to drop.
// The odds of a capsule are the drop chances (from the site's capsule list) of all its
// Mystical trians added together; which trians are Mystical comes from the rarity table.

var (
	builtinByName = map[string][2]string{} // lowercase trian name -> {capsule, rarity}
	builtinInCap  = map[string][]string{}  // capsule -> trian names (lowercase key form)
)

func init() {
	for k, r := range builtinRarities {
		i := strings.Index(k, "|")
		if i < 0 {
			continue
		}
		cap, name := k[:i], k[i+1:]
		if _, dup := builtinByName[name]; !dup {
			builtinByName[name] = [2]string{cap, r}
		}
		builtinInCap[cap] = append(builtinInCap[cap], name)
	}
}

// builtinRarity looks a trian up in the table built from the full collection.
func builtinRarity(capsule, name string) string {
	return builtinRarities[strings.ToLower(capsule)+"|"+strings.ToLower(name)]
}

func capsuleOf(name string) (capsule, rarity string) {
	if v, ok := builtinByName[strings.ToLower(name)]; ok {
		return titleCapsule(v[0]), v[1]
	}
	return "", ""
}

func titleCapsule(lower string) string {
	for _, c := range builtinCapsules {
		if strings.EqualFold(c, lower) {
			return c
		}
	}
	return lower
}

// HuntRow is one capsule in the ranking.
type HuntRow struct {
	Capsule string
	Odds    float64 // chance of a Mystical per capsule opened, in percent (0 = unknown)
	NewOdds float64 // the part of Odds that is Mystical trians you don't own yet
	NewMyst int     // Mystical trians of this capsule you don't own yet
	Chasing bool    // ranked by NewOdds
	Mystics int     // how many Mystical trians the capsule holds
	Known   bool    // Odds came from real drop chances
	Workers int     // how many workers are aimed at it right now
}

// huntRanking ranks capsules, best first. capsules is the list from the site (or the
// built-in list); chances is chanceKey(capsule, trian) -> percent.
func huntRanking(capsules []string, chances map[string]float64, owned map[string]bool) []HuntRow {
	if len(capsules) == 0 {
		capsules = builtinCapsules
	}
	rows := make([]HuntRow, 0, len(capsules))
	for _, c := range capsules {
		row := HuntRow{Capsule: c}
		names := map[string]bool{}
		for _, n := range builtinInCap[strings.ToLower(c)] {
			names[n] = true
		}
		var sum float64
		for k, ch := range chances {
			i := strings.Index(k, "\x00")
			if i < 0 || !strings.EqualFold(k[:i], c) {
				continue
			}
			if builtinRarity(c, k[i+1:]) == "Mystical" {
				sum += ch
				row.Known = true
				if owned != nil && !owned[strings.ToLower(k[i+1:])] {
					row.NewOdds += ch
				}
			}
		}
		for n := range names {
			if builtinRarities[strings.ToLower(c)+"|"+n] == "Mystical" {
				row.Mystics++
				if owned != nil && !owned[n] {
					row.NewMyst++
				}
			}
		}
		row.Odds = sum
		rows = append(rows, row)
	}
	// Smart mode: while some Mystical trian is still missing, rank by the odds of getting
	// one you don't have. Once you own them all, fall back to the plain Mystical odds.
	chase := false
	if owned != nil {
		for _, r := range rows {
			if r.NewOdds > 0 {
				chase = true
			}
		}
	}
	key := func(r HuntRow) float64 {
		if chase {
			return r.NewOdds
		}
		return r.Odds
	}
	for i := range rows {
		rows[i].Chasing = chase
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Known != b.Known {
			return a.Known // real odds beat guesses
		}
		if key(a) != key(b) {
			return key(a) > key(b)
		}
		if a.Odds != b.Odds {
			return a.Odds > b.Odds
		}
		return a.Mystics > b.Mystics
	})
	return rows
}

// huntAssign gives each of n workers a capsule: the best one, or split round-robin across
// the top `spread` capsules.
func huntAssign(rows []HuntRow, spread, n int) []string {
	out := make([]string, n)
	if len(rows) == 0 {
		return out
	}
	if spread < 1 {
		spread = 1
	}
	if spread > len(rows) {
		spread = len(rows)
	}
	for i := range out {
		out[i] = rows[i%spread].Capsule
	}
	return out
}
