package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	superviseEvery = 20 * time.Millisecond
	syncEvery = 30 * time.Millisecond
	fillGap = 10 * time.Millisecond
	sampleEvery = 20 * time.Millisecond
	retryEvery = 30 * time.Millisecond
}

// fakeSite is a tiny stand-in for the site: login, capsule list, account, open.
type fakeSite struct {
	srv     *httptest.Server
	opens   int64
	coins   int64
	logins  int64
	byCap   map[string]*int64
	mystAt  int64         // every Nth open is a Mystical (0 = never)
	failLog int64         // first N logins return no token
	warms   int64         // GET / requests
	latency time.Duration // how long each open takes to answer
	inNow   int64         // opens being answered right now
	idle    int64         // samples where fewer than half the lanes had a request in flight
}

func newFakeSite(coins int64) *fakeSite {
	f := &fakeSite{coins: coins, byCap: map[string]*int64{"OG": new(int64), "Hacker": new(int64), "Hat": new(int64)}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			n := atomic.AddInt64(&f.logins, 1)
			if n <= atomic.LoadInt64(&f.failLog) {
				w.Write([]byte(`{"ok":true}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"token": testJWT})
		case "/data/trians":
			if r.Header.Get("authorization") != testJWT {
				w.Write([]byte(`{"message":"unauthorized"}`))
				return
			}
			w.Write([]byte(`{"ValuesnCapsules":[
			 {"name":"OG","trians":[{"name":"Plain","chance":"90%"},{"name":"Gold Tri","chance":"0.5%"}]},
			 {"name":"Hacker","trians":[{"name":"Dark Web","chance":"2%"},{"name":"Script","chance":"50%"}]},
			 {"name":"Hat","trians":[{"name":"Diamond Hat","chance":"0.2%"},{"name":"Cap","chance":"60%"}]}]}`))
		case "/data/user":
			c := atomic.LoadInt64(&f.coins)
			w.Write([]byte(`{"user":{"tokens":` + itoa(c) + `,"opened":7,"trians":[{"trian":"Dark Web","quantity":2},{"trian":"Cap","quantity":5}]}}`))
		case "/api/open":
			if r.Header.Get("authorization") != testJWT {
				w.Write([]byte(`{"message":"unauthorized"}`))
				return
			}
			if atomic.AddInt64(&f.coins, -25) < 0 {
				atomic.AddInt64(&f.coins, 25)
				w.Write([]byte(`{"error":"You don't have enough tokens"}`))
				return
			}
			var b map[string]string
			json.NewDecoder(r.Body).Decode(&b)
			if f.latency > 0 {
				atomic.AddInt64(&f.inNow, 1)
				time.Sleep(f.latency)
				atomic.AddInt64(&f.inNow, -1)
			}
			n := atomic.AddInt64(&f.opens, 1)
			if c := f.byCap[b["capsule"]]; c != nil {
				atomic.AddInt64(c, 1)
			}
			rar := "Rare"
			if f.mystAt > 0 && n%f.mystAt == 0 {
				rar = "Mystical"
			}
			json.NewEncoder(w).Encode(map[string]any{"rarity": rar, "trian": "X", "new": false})
		case "/api/addauction":
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if _, isNum := b["amount"].(float64); !isNum {
				w.Write([]byte(`{"ok":true}`))
				return
			}
			w.WriteHeader(400)
			w.Write([]byte(`{"error":"invalid amount"}`))
		case "/":
			atomic.AddInt64(&f.warms, 1)
			w.Write([]byte("ok"))
		default:
			w.WriteHeader(404)
		}
	}))
	return f
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func newTestHub(t *testing.T, f *fakeSite) *Hub {
	t.Helper()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	h := NewHub(dataDir())
	h.primary.cfg.BaseURL = f.srv.URL
	h.primary.cfg.DelayMs = 1
	h.primary.cfg.Username, h.primary.cfg.Password = "u", "p"
	h.primary.cfg.LoginPath, h.primary.cfg.LoginField = "/api/login", "name"
	h.primary.errBackoff = time.Millisecond
	t.Cleanup(h.Close)
	return h
}

func waitFor(t *testing.T, d time.Duration, what string, f func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestHubRunsManyWorkersAndLanes(t *testing.T) {
	f := newFakeSite(1_000_000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) { c.Workers = 4; c.InFlight = 3; c.Capsule = "OG"; c.Mode = "fixed" })
	h.Start()
	waitFor(t, 3*time.Second, "4 workers running", func() bool { return h.Snapshot().WorkersRunning == 4 })
	waitFor(t, 3*time.Second, "pulls", func() bool { return atomic.LoadInt64(&f.opens) > 100 })
	h.Stop()
	time.Sleep(100 * time.Millisecond)
	if s := h.Snapshot(); s.Workers != 4 || s.InFlight != 3 || s.Opened < 100 {
		t.Fatalf("snapshot: workers=%d inflight=%d opened=%d", s.Workers, s.InFlight, s.Opened)
	}
	if atomic.LoadInt64(f.byCap["OG"]) == 0 || atomic.LoadInt64(f.byCap["Hat"]) != 0 {
		t.Fatalf("fixed mode must stay on OG")
	}
}

func TestStopsExactlyWhenCoinsRunOut(t *testing.T) {
	f := newFakeSite(25 * 40) // exactly 40 capsules
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) { c.Workers = 3; c.InFlight = 4; c.Capsule = "OG"; c.Mode = "fixed" })
	h.Start()
	waitFor(t, 3*time.Second, "all coins spent", func() bool { return atomic.LoadInt64(&f.opens) >= 40 })
	time.Sleep(300 * time.Millisecond)
	h.Stop()
	if n := atomic.LoadInt64(&f.opens); n != 40 {
		t.Fatalf("opened %d, want exactly 40", n)
	}
	if left := atomic.LoadInt64(&f.coins); left != 0 {
		t.Fatalf("coins left %d", left)
	}
	if s := h.Snapshot(); s.PacksLeft != 0 {
		t.Fatalf("packs left %d", s.PacksLeft)
	}
}

func TestHuntFocusesBestCapsule(t *testing.T) {
	f := newFakeSite(1_000_000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) {
		c.Workers = 2
		c.InFlight = 1
		c.Capsule = "OG"
		c.Mode = "hunt"
		c.HuntSpread = 1
		c.HuntMissing = false
	})
	if err := h.primary.LoginRetry(nil); err != nil {
		t.Fatal(err)
	}
	if err := h.primary.LoadCapsules(); err != nil {
		t.Fatal(err)
	}
	h.Start()
	waitFor(t, 3*time.Second, "pulls", func() bool { return atomic.LoadInt64(&f.opens) > 50 })
	h.Stop()
	s := h.Snapshot()
	if len(s.Hunt) == 0 || s.Hunt[0].Capsule != "Hacker" {
		t.Fatalf("ranking: %+v", s.Hunt)
	}
	if atomic.LoadInt64(f.byCap["Hacker"]) == 0 || atomic.LoadInt64(f.byCap["Hat"]) != 0 || atomic.LoadInt64(f.byCap["OG"]) != 0 {
		t.Fatalf("hunt must stay on Hacker: Hacker=%d Hat=%d OG=%d", *f.byCap["Hacker"], *f.byCap["Hat"], *f.byCap["OG"])
	}
}

func TestHuntSplitsAcrossTopCapsules(t *testing.T) {
	rows := huntRanking([]string{"OG", "Hacker", "Hat"}, map[string]float64{
		chanceKey("Hacker", "Dark Web"): 2, chanceKey("Hat", "Diamond Hat"): 0.2, chanceKey("OG", "Gold Tri"): 0.5,
	}, nil)
	if rows[0].Capsule != "Hacker" {
		t.Fatalf("rows = %+v", rows)
	}
	got := huntAssign(rows, 2, 5)
	want := []string{rows[0].Capsule, rows[1].Capsule, rows[0].Capsule, rows[1].Capsule, rows[0].Capsule}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("assign = %v want %v", got, want)
	}
}

func TestMysticalAdvanceAndMeasuredOdds(t *testing.T) {
	f := newFakeSite(1_000_000)
	f.mystAt = 10
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) { c.Workers = 1; c.InFlight = 1; c.Capsule = "OG"; c.Mode = "advance" })
	h.primary.LoginRetry(nil)
	h.primary.LoadCapsules()
	h.Start()
	waitFor(t, 3*time.Second, "capsule to advance", func() bool { return h.cfg().Capsule != "OG" })
	waitFor(t, 3*time.Second, "30 pulls", func() bool { return atomic.LoadInt64(&f.opens) > 30 })
	h.Stop()
	s := h.Snapshot()
	if s.Mystical < 2 || s.PacksPerMystical < 8 || s.PacksPerMystical > 16 {
		t.Fatalf("mystical=%d packs/mystical=%.1f", s.Mystical, s.PacksPerMystical)
	}
	if s.CoinsPerMystical != s.PacksPerMystical*25 {
		t.Fatalf("coins/mystical = %v", s.CoinsPerMystical)
	}
}

func TestLoginRetriesUntilTokenComes(t *testing.T) {
	f := newFakeSite(1000)
	f.failLog = 3
	defer f.srv.Close()
	h := newTestHub(t, f)
	if err := h.primary.LoginRetry(nil); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(&f.logins); n != 4 {
		t.Fatalf("logins = %d", n)
	}
}

func TestInventoryAndCoinsFromAccount(t *testing.T) {
	f := newFakeSite(2500)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.primary.LoginRetry(nil)
	h.primary.LoadCapsules()
	if err := h.primary.SyncInventory(); err != nil {
		t.Fatal(err)
	}
	s := h.Snapshot()
	if !s.HasAcct || s.Coins != 2500 || s.PacksLeft != 100 || s.AcctOpened != 7 {
		t.Fatalf("acct: %+v", s)
	}
	if len(s.Inv) != 2 || s.Inv[0].Name != "Dark Web" || s.Inv[0].Rarity != "Mystical" || s.Inv[0].Qty != 2 || s.InvTotal != 7 {
		t.Fatalf("inv: %+v", s.Inv)
	}
}

func TestAuctionRetriesWithStrings(t *testing.T) {
	f := newFakeSite(1000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.primary.LoginRetry(nil)
	if err := h.primary.AddAuction("Cap", 2, 10, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.primary.AddAuction("", 2, 10, 0); err == nil {
		t.Fatal("expected error for empty trian")
	}
}

func TestPullLogKeepsNotablePullsAndWritesCSV(t *testing.T) {
	f := newFakeSite(1000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.logPull("OG", openResp{Rarity: "Rare", Trian: "Plain"})            // not notable
	h.logPull("OG", openResp{Rarity: "Rare", Trian: "Fresh", New: true}) // new trian
	h.logPull("Hacker", openResp{Rarity: "Mystical", Trian: "Dark Web"}) // Epic or better
	h.logPull("Hat", openResp{Rarity: "Uncommon", Trian: "Cap"})         // not notable
	got := h.recentLog(10)
	if len(got) != 2 || got[0].Trian != "Dark Web" || got[1].Trian != "Fresh" {
		t.Fatalf("log = %+v", got)
	}
	b, err := os.ReadFile(h.logPath())
	if err != nil || !strings.Contains(string(b), "Mystical,Dark Web,Hacker,no") || !strings.HasPrefix(string(b), "time,rarity") {
		t.Fatalf("csv: %v\n%s", err, b)
	}
}

func TestMissingTrackerCountsOwnedTrians(t *testing.T) {
	if len(builtinTrians) != 117 {
		t.Fatalf("expected 117 trians, have %d", len(builtinTrians))
	}
	inv := []InvItem{{Name: "dark web", Qty: 3}, {Name: "Diamond Hat", Qty: 1}, {Name: "Not A Real Trian", Qty: 9}}
	got, missing, owned := collection(inv)
	if got != 2 || len(missing) != 115 || !owned["dark web"] {
		t.Fatalf("collected=%d missing=%d", got, len(missing))
	}
	for _, m := range missing {
		if m.Qty != 0 || m.Name == "Dark Web" || m.Name == "Diamond Hat" {
			t.Fatalf("bad missing entry %+v", m)
		}
	}
	if missing[0].Rarity != "Mystical" {
		t.Fatalf("missing list should lead with the rarest, got %+v", missing[0])
	}
}

func TestHuntChasesMissingMysticalsThenFallsBack(t *testing.T) {
	ch := map[string]float64{chanceKey("Hacker", "Dark Web"): 3, chanceKey("Hat", "Diamond Hat"): 1}
	caps := []string{"Hacker", "Hat"}
	if r := huntRanking(caps, ch, nil); r[0].Capsule != "Hacker" {
		t.Fatalf("plain ranking: %+v", r)
	}
	// own Hacker's Mystical but not Hat's: chase Hat
	r := huntRanking(caps, ch, map[string]bool{"dark web": true})
	if r[0].Capsule != "Hat" || !r[0].Chasing || r[0].NewMyst != 1 || r[0].NewOdds != 1 {
		t.Fatalf("chase ranking: %+v", r)
	}
	// own both: fall back to the plain odds
	r = huntRanking(caps, ch, map[string]bool{"dark web": true, "diamond hat": true})
	if r[0].Capsule != "Hacker" || r[0].Chasing {
		t.Fatalf("fallback ranking: %+v", r)
	}
}

func TestStopsAfterGoalMysticals(t *testing.T) {
	f := newFakeSite(1_000_000)
	f.mystAt = 6
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) {
		c.Workers = 1
		c.InFlight = 1
		c.Capsule = "OG"
		c.Mode = "fixed"
		c.StopAfterMystical = 2
	})
	var sounds int64
	alertHook = func(string) { atomic.AddInt64(&sounds, 1) }
	defer func() { alertHook = nil }()
	h.Start()
	waitFor(t, 3*time.Second, "goal reached", func() bool { return !h.Snapshot().Running })
	time.Sleep(150 * time.Millisecond)
	n := atomic.LoadInt64(&f.opens)
	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt64(&f.opens) != n {
		t.Fatal("kept opening after the goal")
	}
	if s := h.Snapshot(); s.MysticalRun < 2 || s.Mystical < 2 {
		t.Fatalf("run=%d total=%d", s.MysticalRun, s.Mystical)
	}
	if atomic.LoadInt64(&sounds) == 0 {
		t.Fatal("no alert sound for the Mystical")
	}
	found := false
	for i := 0; i < 20; i++ {
		if title, _, ok := h.primary.takeBalloon(); ok && title == "Goal reached" {
			found = true
		}
	}
	if !found {
		t.Fatal("no Goal reached notification")
	}
}

func TestAlertsForLowAndOutOfCoins(t *testing.T) {
	f := newFakeSite(25 * 30) // 30 packs
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) {
		c.Workers = 1
		c.InFlight = 1
		c.Capsule = "OG"
		c.Mode = "fixed"
		c.AlertLowPacks = 20
		c.AlertSound = false
	})
	h.primary.LoginRetry(nil)
	h.primary.LoadCapsules()
	h.primary.SyncInventory()
	h.Start()
	waitFor(t, 4*time.Second, "all packs used", func() bool { return atomic.LoadInt64(&f.opens) >= 30 })
	time.Sleep(300 * time.Millisecond)
	titles := map[string]int{}
	for i := 0; i < 20; i++ {
		if title, _, ok := h.primary.takeBalloon(); ok {
			titles[title]++
		}
	}
	h.Stop()
	if titles["Coins running low"] != 1 || titles["Out of coins"] != 1 {
		t.Fatalf("alerts = %v", titles)
	}
}

func TestBatchedCoinsStillStopExactly(t *testing.T) {
	const caps = 2*8*64 + 13 // big enough that batches are handed out at first
	f := newFakeSite(25 * caps)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) { c.Workers = 3; c.InFlight = 6; c.DelayMs = 0; c.Capsule = "OG"; c.Mode = "fixed" })
	h.Start()
	waitFor(t, 8*time.Second, "all coins spent", func() bool { return atomic.LoadInt64(&f.opens) >= caps })
	time.Sleep(400 * time.Millisecond)
	h.Stop()
	if n := atomic.LoadInt64(&f.opens); n != caps {
		t.Fatalf("opened %d, want exactly %d", n, caps)
	}
	if left := atomic.LoadInt64(&f.coins); left != 0 {
		t.Fatalf("coins left %d", left)
	}
}

func TestLeaseAccounting(t *testing.T) {
	p := newCoinPool()
	p.set(25 * 1000)
	n := p.takeLease(25)
	if n != leaseBatch {
		t.Fatalf("batch = %d", n)
	}
	if p.get() != 25*(1000-leaseBatch) {
		t.Fatalf("balance %d", p.get())
	}
	p.done(25, false) // first capsule opened
	p.returnLease(25, n-1)
	if p.get() != 25*(1000-1) {
		t.Fatalf("after return %d", p.get())
	}
	if p.othersHold() {
		t.Fatal("nothing should still be held")
	}
	small := newCoinPool()
	small.set(25 * 100)
	if got := small.takeLease(25); got != 1 {
		t.Fatalf("small balance must hand out single capsules, got %d", got)
	}
}

func TestWarmOpensConnections(t *testing.T) {
	f := newFakeSite(1000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.primary.Warm(6)
	if n := atomic.LoadInt64(&f.warms); n != 6 {
		t.Fatalf("warm requests = %d", n)
	}
}

func TestMeasureLimitsSpendsNothing(t *testing.T) {
	old := measureStep
	measureStep = 120 * time.Millisecond
	defer func() { measureStep = old }()
	f := newFakeSite(5000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	if err := h.primary.LoginRetry(nil); err != nil {
		t.Fatal(err)
	}
	h.MeasureLimits()
	waitFor(t, 20*time.Second, "test finished", func() bool {
		s := h.Snapshot()
		return !s.SpeedBusy && strings.HasPrefix(s.SpeedMsg, "Set ")
	})
	c := h.cfg()
	if c.Workers < 1 || c.Workers > maxWorkers || c.InFlight < 1 || c.InFlight > maxInFlight {
		t.Fatalf("settings out of range: %+v", c)
	}
	if atomic.LoadInt64(&f.opens) != 0 || atomic.LoadInt64(&f.coins) != 5000 {
		t.Fatalf("test must not spend coins: opens=%d coins=%d", f.opens, f.coins)
	}
}

func TestBestLevelAndSettings(t *testing.T) {
	pts := []SpeedPoint{{2, 100, 0}, {4, 190, 0}, {8, 380, 0}, {16, 400, 0}, {32, 410, 0}, {64, 300, 12}}
	b, ok := bestLevel(pts)
	if !ok || b.Level != 8 {
		t.Fatalf("best = %+v", b)
	}
	if _, ok := bestLevel([]SpeedPoint{{2, 50, 40}}); ok {
		t.Fatal("an erroring level must not count")
	}
	for _, lv := range []int{1, 8, 9, 100, 256, 1000} {
		w, f := settingsFor(lv)
		if w < 1 || w > maxWorkers || f < 1 || f > maxInFlight {
			t.Fatalf("level %d -> %d x %d", lv, w, f)
		}
	}
	if w, f := settingsFor(40); w*f < 40 {
		t.Fatalf("40 -> %d x %d", w, f)
	}
}

func TestSwitchAccountsAndRemove(t *testing.T) {
	f := newFakeSite(5000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.Login("alice", "pw1")
	waitFor(t, 5*time.Second, "alice token", func() bool { return h.primary.hasToken() })
	h.Login("bob", "pw2")
	waitFor(t, 5*time.Second, "both saved", func() bool { l, _ := h.Accounts(); return len(l) == 2 })
	if _, idx := h.Accounts(); idx != 1 {
		t.Fatalf("active index = %d", idx)
	}
	h.SwitchAccount(1) // wraps to alice
	waitFor(t, 5*time.Second, "switched to alice", func() bool {
		c := h.cfg()
		return c.Username == "alice" && c.Password == "pw1" && h.primary.hasToken()
	})
	if s := h.Snapshot(); s.AcctIdx != 0 || len(s.AcctNames) != 2 {
		t.Fatalf("snapshot accounts: %+v %d", s.AcctNames, s.AcctIdx)
	}
	h.RemoveAccount()
	waitFor(t, 5*time.Second, "bob after remove", func() bool { return h.cfg().Username == "bob" && h.primary.hasToken() })
	if l, _ := h.Accounts(); len(l) != 1 || l[0].Name != "bob" {
		t.Fatalf("list after remove: %+v", l)
	}
	h.Login("BOB", "pw3") // same name: updates, doesn't duplicate
	waitFor(t, 5*time.Second, "bob updated", func() bool { l, _ := h.Accounts(); return len(l) == 1 && l[0].Password == "pw3" })
}

func TestSwitchRestartsOpeningWhenItWasOn(t *testing.T) {
	f := newFakeSite(1_000_000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) { c.Capsule = "OG"; c.Mode = "fixed"; c.Workers = 1; c.InFlight = 2 })
	h.Login("alice", "pw1")
	h.Login("bob", "pw2")
	waitFor(t, 5*time.Second, "token", func() bool { return h.primary.hasToken() })
	h.Start()
	waitFor(t, 5*time.Second, "opening", func() bool { return atomic.LoadInt64(&f.opens) > 5 })
	before := atomic.LoadInt64(&f.opens)
	h.SwitchAccount(-1)
	waitFor(t, 8*time.Second, "opening again after switch", func() bool { return atomic.LoadInt64(&f.opens) > before+20 })
	h.Stop()
}

func TestHistoryRecordsAndPersists(t *testing.T) {
	f := newFakeSite(25 * 60)
	f.mystAt = 10
	defer f.srv.Close()
	dir := t.TempDir()
	h := NewHub(dir)
	h.primary.noSave = true
	h.primary.cfg.BaseURL = f.srv.URL
	h.update(func(c *Config) {
		c.Username = "u"
		c.Password = "p"
		c.Capsule = "OG"
		c.Mode = "fixed"
		c.Workers = 1
		c.InFlight = 2
		c.DelayMs = 0
	})
	h.Start()
	waitFor(t, 5*time.Second, "all opened", func() bool { return atomic.LoadInt64(&f.opens) >= 60 })
	time.Sleep(200 * time.Millisecond)
	h.Stop()
	h.recordCoins(1234)
	h.Close()
	h2 := NewHub(dir)
	defer h2.Close()
	hist := h2.historySnapshot()
	if len(hist) == 0 {
		t.Fatal("history was not saved")
	}
	pulls, myst := 0, 0
	for _, b := range hist {
		pulls += b.Pulls
		myst += b.Mystical
	}
	if pulls != 60 || myst != 6 {
		t.Fatalf("pulls=%d mystical=%d", pulls, myst)
	}
	if hist[len(hist)-1].Coins != 1234 {
		t.Fatalf("coins = %d", hist[len(hist)-1].Coins)
	}
}

func TestHistSeriesGroupsAndCarries(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 11, 18, 15, 30, 0, 0, loc)
	h := func(day, hr int) int64 { return time.Date(2026, 11, day, hr, 0, 0, 0, loc).Unix() }
	hist := []HistBucket{
		{T: h(10, 4), Pulls: 5, Coins: 900},
		{T: h(18, 13), Pulls: 100, Mystical: 2, Coins: 800},
		{T: h(18, 15), Pulls: 40, Chroma: 1, Coins: -1},
		{T: h(17, 22), Pulls: 7, Coins: 850},
	}
	d := histSeries(hist, now, 0)
	if len(d) != 24 || d[23].Label != "15:00" || d[23].Pulls != 40 || d[23].Chroma != 1 {
		t.Fatalf("24h: %+v", d[22:])
	}
	if d[21].Pulls != 100 || d[21].Mystical != 2 {
		t.Fatalf("13:00 bucket: %+v", d[21])
	}
	if d[22].Coins != 800 || d[23].Coins != 800 {
		t.Fatalf("coins must carry forward: %d %d", d[22].Coins, d[23].Coins)
	}
	w := histSeries(hist, now, 1)
	if len(w) != 7 || w[6].Pulls != 140 || w[5].Pulls != 7 || w[6].Label != "Nov 18" {
		t.Fatalf("7d: %+v", w)
	}
	m := histSeries(hist, now, 2)
	if len(m) != 30 || m[29].Pulls != 140 {
		t.Fatalf("30d: %+v", m[27:])
	}
	if m[21].Label != "Nov 10" || m[21].Coins != 900 || m[22].Coins != 900 || m[20].Coins != -1 {
		t.Fatalf("30d coins: %+v", m[20:23])
	}
}

// With many lanes and a slow answer, requests must keep flowing: the number in flight stays near the
// number of lanes (no wave of requests, then a pause while they all finish and the next wave starts).
func TestRequestsKeepFlowingWithoutPauses(t *testing.T) {
	f := newFakeSite(1_000_000)
	f.latency = 25 * time.Millisecond
	h := newTestHub(t, f)
	defer h.Close()
	h.primary.cfg.Capsule = "OG"
	h.primary.cfg.InFlight, h.primary.cfg.Workers = 24, 1
	h.primary.cfg.DelayMs = 0.000001
	h.Start()
	waitFor(t, 5*time.Second, "requests flowing", func() bool { return atomic.LoadInt64(&f.opens) > 100 })
	low, samples := 0, 0
	for i := 0; i < 150; i++ {
		time.Sleep(2 * time.Millisecond)
		samples++
		if atomic.LoadInt64(&f.inNow) < 12 {
			low++
		}
	}
	h.Stop()
	if low*10 > samples { // more than 10% of the time the pipe was less than half full
		t.Fatalf("pipe was less than half full in %d of %d samples", low, samples)
	}
}
