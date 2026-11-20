package main

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleSnapshot() Snapshot {
	var inv []InvItem
	names := []string{"Dark Web", "Diamond Hat", "Meteor", "Pink Knight", "Monkey", "Rocket", "Pizza", "Crown", "Ghost", "Robot", "Fire", "Chess King"}
	rar := []string{"Mystical", "Mystical", "Mystical", "Mystical", "Chroma", "Legendary", "Epic", "Epic", "Rare", "Rare", "Uncommon", "Uncommon"}
	caps := []string{"Hacker", "Hat", "Space", "Chess", "Emoji", "Space", "Food", "Chess", "Emoji", "Hacker", "Sports", "OG"}
	for j := range names {
		inv = append(inv, InvItem{Name: names[j], Rarity: rar[j], Capsule: caps[j], Qty: 53 - j*4})
	}
	for j := 0; j < 12; j++ {
		inv = append(inv, InvItem{Name: "Sample " + string(rune('A'+j)), Rarity: "Rare", Capsule: "OG", Qty: j + 1})
	}
	series := []int{}
	for j := 0; j < 120; j++ {
		series = append(series, j*95+int(30*float64(j%7)))
	}
	_, missing, owned := collection(inv)
	rows := huntRanking(builtinCapsules, map[string]float64{
		chanceKey("Hacker", "Dark Web"): 3.2, chanceKey("Hat", "Diamond Hat"): 1.1, chanceKey("Space", "Meteor"): 2.4,
		chanceKey("Chess", "Pink Knight"): 0.9, chanceKey("Food", "Golden Pizza"): 0.4,
	}, owned)
	collected := len(builtinTrians) - len(missing)
	now := time.Now()
	logEv := []PullEvent{
		{now.Add(-12 * time.Second), "Dark Web", "Mystical", "Hacker", false},
		{now.Add(-95 * time.Second), "Rocket", "Legendary", "Space", true},
		{now.Add(-3 * time.Minute), "Monkey", "Chroma", "Emoji", false},
		{now.Add(-7 * time.Minute), "Crown", "Epic", "Chess", false},
		{now.Add(-11 * time.Minute), "Pizza", "Epic", "Food", true},
		{now.Add(-18 * time.Minute), "Meteor", "Mystical", "Space", false},
		{now.Add(-26 * time.Minute), "Ghost", "Epic", "Emoji", false},
	}
	rows[0].Workers, rows[1].Workers = 3, 2
	return Snapshot{
		Running: true, Capsule: "Hacker", Capsules: builtinCapsules, Mode: "hunt", HuntSpread: 2, Workers: 5, WorkersRunning: 5, InFlight: 3,
		CoinCost: 25, Opened: 11840, Counts: map[string]int{"Uncommon": 6120, "Rare": 3900, "Epic": 1500, "Legendary": 260, "Chroma": 48, "Mystical": 12},
		PerMin: 5400, Series: series, HasAcct: true, Coins: 482500, AcctOpened: 52310, PacksLeft: 19300, Mystical: 12, SinceMystical: 311,
		PacksPerMystical: 986, CoinsPerMystical: 24650, Hunt: rows, Inv: inv, InvTotal: 640, StartedAgo: 2195, TokenDesc: "crysticat, expires 9:41 PM",
		Username: "crysticat", DelayMs: 2, Info: "Logged in as crysticat",
		Log: logEv, Collected: collected, TotalTrians: len(builtinTrians), Missing: missing,
		StopAfter: 3, MysticalRun: 1, AlertSound: true, HuntMissing: true,
		Hist: sampleHist(), SpeedMsg: "Set 6 workers x 8 (~1,240 req/s)", AcctNames: []string{"crysticat", "alt_two", "alt_three"}, AcctIdx: 0,
	}
}

// PREVIEW_DIR=/some/dir go test -run TestScenePreview  writes one PNG + text list per tab.
func TestScenePreview(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR to render previews")
	}
	os.MkdirAll(dir, 0o755)
	for tab := 0; tab < 11; tab++ {
		applyTheme("dark")
		st := NewUIState()
		st.Settle = true
		st.Tab, st.StartupOn = tab, true
		st.Wall = time.Date(2026, 11, 18, 15, 30, 0, 0, time.UTC)
		st.HistRange = 0
		st.Sel = "Dark Web"
		if tab == 6 { // the Missing view of the Trians tab
			st.Tab, st.TriansView = 0, 1
		}
		st.Hover = "start"
		s := sampleSnapshot()
		switch {
		case tab >= 7 && tab <= 9: // Settings page in dark / light / blue
			st.Tab, st.Settings = 0, true
			s.Theme = themeNames[tab-7]
			applyTheme(s.Theme)
			st.SetMsg = "Saved: C:\\Users\\you\\Desktop\\TrianguletOpener-backup-2026-11-20.tribackup"
		case tab == 10: // the Trians tab in the light theme
			st.Tab = 0
			applyTheme("light")
		}
		c := NewCanvas(winW, winH)
		f := DrawScene(c, s, st)
		applyTheme("dark")
		img := image.NewRGBA(image.Rect(0, 0, winW, winH))
		copy(img.Pix, c.Pix)
		name := filepath.Join(dir, "tab"+itoaInt(tab))
		fh, _ := os.Create(name + ".png")
		png.Encode(fh, img)
		fh.Close()
		b, _ := json.Marshal(f.Texts)
		os.WriteFile(name+".json", b, 0o644)
		e, _ := json.Marshal(f.Edits)
		os.WriteFile(name+".edits", e, 0o644)
	}
}

func TestSceneHitsAndAnimation(t *testing.T) {
	st := NewUIState()
	c := NewCanvas(winW, winH)
	f := DrawScene(c, sampleSnapshot(), st)
	found := map[string]bool{}
	for _, h := range f.Hits {
		found[h.ID] = true
	}
	for _, id := range []string{"login", "start", "hide", "mode:0", "mode:2", "spread:1", "workers+", "inflight-", "startup", "boot", "tab:2", "refresh", "tv:1", "speedtest", "acct>", "acct<", "acctnew", "acct-"} {
		if id == "spread:1" {
			continue // only shown in Hunt mode, which this sample is in
		}
		if !found[id] {
			t.Errorf("missing hit area %q", id)
		}
	}
	if !found["spread:1"] {
		t.Errorf("hunt options not drawn")
	}
	// numbers count up from zero and settle
	if st.disp["n:PER MINUTE"] != 0 {
		t.Fatalf("count-up should start at zero")
	}
	snap := sampleSnapshot()
	for i := 0; i < 40; i++ {
		st.Step(0.1)
		DrawScene(c, snap, st)
	}
	if v := st.disp["n:PER MINUTE"]; v < 5390 {
		t.Fatalf("count-up did not reach its target: %v", v)
	}
	if st.Animating() && !st.running {
		t.Fatalf("still animating after settling")
	}
}

func TestIdleDoesNotAnimate(t *testing.T) {
	st := NewUIState()
	c := NewCanvas(winW, winH)
	s := sampleSnapshot()
	s.Running = false
	for i := 0; i < 60; i++ {
		DrawScene(c, s, st)
		st.Step(0.1)
	}
	if st.Animating() {
		t.Fatalf("an idle window must stop animating (CPU use)")
	}
}

// sampleHist makes a few days of believable hourly activity ending at 2026-11-18 15:00 UTC.
func sampleHist() []HistBucket {
	end := time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)
	var out []HistBucket
	coins := 90000
	for i := 24 * 8; i >= 0; i-- {
		tm := end.Add(-time.Duration(i) * time.Hour)
		if tm.Hour() >= 2 && tm.Hour() < 7 {
			continue // asleep
		}
		n := 900 + (i*379)%2300
		b := HistBucket{T: tm.Unix(), Pulls: n}
		if i%7 == 0 {
			b.Mystical = 1
		}
		if i%11 == 3 {
			b.Chroma = 1
		}
		coins = 420000 + (i*7919)%110000 + (24*8-i)*60
		b.Coins = coins
		out = append(out, b)
	}
	return out
}
