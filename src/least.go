package main

import "math/rand"

// leastTarget picks the trian to go for: the one you own the fewest of (0 if missing).
// Among trians tied for the lowest count it picks one at random, and keeps the previous
// pick for as long as it is still tied, so the workers don't jump around.
// If only is not nil, only those trian names (any case) are considered.
func leastTarget(inv []InvItem, only []string, prev string) (name, capsule string) {
	qty := map[string]int{}
	for _, it := range inv {
		qty[lowerKey(it.Name)] += it.Qty
	}
	var allow map[string]bool
	if only != nil {
		allow = map[string]bool{}
		for _, n := range only {
			allow[lowerKey(n)] = true
		}
	}
	best := -1
	var ties []builtinTrian
	for _, t := range builtinTrians {
		k := lowerKey(t.Name)
		if allow != nil && !allow[k] {
			continue
		}
		q := qty[k]
		switch {
		case best < 0 || q < best:
			best, ties = q, []builtinTrian{t}
		case q == best:
			ties = append(ties, t)
		}
	}
	if len(ties) == 0 {
		return "", ""
	}
	for _, t := range ties {
		if t.Name == prev {
			return t.Name, t.Capsule
		}
	}
	t := ties[rand.Intn(len(ties))]
	return t.Name, t.Capsule
}
