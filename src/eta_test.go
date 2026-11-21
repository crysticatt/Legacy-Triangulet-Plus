package main

import (
	"math"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol*math.Max(1, math.Abs(b)) }

func TestExpectedPacksKnownCases(t *testing.T) {
	if got := expectedPacks([]float64{0.01}); !near(got, 100, 1e-9) {
		t.Fatalf("single: %v", got)
	}
	// two equal items: E = 1/p * (1 + 1/2) = 150
	if got := expectedPacks([]float64{0.01, 0.01}); !near(got, 150, 0.01) {
		t.Fatalf("two equal: %v", got)
	}
	// ten equal items p=0.1: coupon collector 10*H10 = 29.2897 (Poissonized ~ the same)
	ps := make([]float64, 10)
	for i := range ps {
		ps[i] = 0.1
	}
	if got := expectedPacks(ps); !near(got, 10*2.928968, 0.01) {
		t.Fatalf("ten equal: %v", got)
	}
	// unequal: p1=0.5, p2=0.01 -> 1/p1 + 1/p2 - 1/(p1+p2) = 2+100-1.96078
	if got := expectedPacks([]float64{0.5, 0.01}); !near(got, 100.0392, 0.01) {
		t.Fatalf("unequal: %v", got)
	}
	if expectedPacks(nil) != 0 || expectedPacks([]float64{0}) != 0 {
		t.Fatal("nothing missing should be 0")
	}
}

func TestETAFromGroupsAndUnknownOdds(t *testing.T) {
	chances := map[string]float64{
		chanceKey("Hacker", "Dark Web"): 2, chanceKey("Hat", "Diamond Hat"): 0.5, chanceKey("Hat", "Cap"): 60,
	}
	missing := []InvItem{
		{Name: "Dark Web", Capsule: "Hacker", Rarity: "Mystical"},
		{Name: "Diamond Hat", Capsule: "Hat", Rarity: "Mystical"},
		{Name: "Cap", Capsule: "Hat", Rarity: "Common"},
		{Name: "Ghost Item", Capsule: "Hat", Rarity: "Mystical"}, // no odds known
	}
	g := etaFrom(missing, chances)
	if g[0].Name != "Missing Mysticals" || g[0].Missing != 2 || g[0].NoOdds != 1 || g[0].Hardest != "Diamond Hat" {
		t.Fatalf("%+v", g[0])
	}
	want := 1/0.02 + 1/0.005 // different capsules add up
	if !near(g[0].Packs, want, 1e-6) {
		t.Fatalf("mystical packs %v want %v", g[0].Packs, want)
	}
	if g[1].Missing != 3 || g[1].Packs <= g[0].Packs {
		t.Fatalf("whole collection: %+v", g[1])
	}
}

func TestLuckPercentAndExpected(t *testing.T) {
	// exactly as expected -> around the middle; far above -> near 100; far below -> near 0
	if p := luckPct(10, 10, 9.9); p < 40 || p > 65 {
		t.Fatalf("on target: %v", p)
	}
	if p := luckPct(25, 10, 9.9); p < 99 {
		t.Fatalf("lucky: %v", p)
	}
	if p := luckPct(0, 10, 9.9); p > 1 {
		t.Fatalf("unlucky: %v", p)
	}
	if p := luckPct(2000, 1800, 1700); p < 90 {
		t.Fatalf("normal branch: %v", p)
	}
	chances := map[string]float64{chanceKey("Hacker", "Dark Web"): 2, chanceKey("Hacker", "Script"): 50}
	if pm, ok := rarityChance(chances, "hacker", "Mystical"); !ok || !near(pm, 0.02, 1e-9) {
		t.Fatalf("rarityChance %v %v", pm, ok)
	}
	info := luckFrom(map[string]*capOdds{"hacker": {Pulls: 1000, ByRarity: map[string]int{"Mystical": 30}}, "nochance": {Pulls: 500}}, chances)
	if info.Pulls != 1000 || info.AllPulls != 1500 || info.Rows[0].Rarity != "Mystical" || info.Rows[0].Actual != 30 || !near(info.Rows[0].Expected, 20, 1e-9) {
		t.Fatalf("%+v", info)
	}
}

func TestHuntBlendsMeasuredOdds(t *testing.T) {
	chances := map[string]float64{chanceKey("Hacker", "Dark Web"): 2, chanceKey("Hat", "Diamond Hat"): 1}
	caps := []string{"Hacker", "Hat"}
	base := huntRanking(caps, chances, nil)
	if base[0].Capsule != "Hacker" {
		t.Fatalf("advertised order: %+v", base)
	}
	// Hacker measured much worse than advertised, Hat much better -> Hat now leads
	m := map[string][2]int{"hacker": {20000, 20}, "hat": {20000, 600}}
	rows := huntRankingM(caps, chances, nil, m)
	if rows[0].Capsule != "Hat" || rows[0].Pulls != 20000 || rows[0].Measured < 2.9 || rows[0].Measured > 3.1 {
		t.Fatalf("%+v", rows)
	}
	if rows[0].Odds <= 1 || rows[0].Odds >= 3 {
		t.Fatalf("blend should sit between advertised and measured: %v", rows[0].Odds)
	}
	// a handful of pulls barely moves it
	few := huntRankingM(caps, chances, nil, map[string][2]int{"hat": {10, 5}})
	if few[0].Capsule != "Hacker" {
		t.Fatalf("10 pulls should not flip the order: %+v", few)
	}
}
