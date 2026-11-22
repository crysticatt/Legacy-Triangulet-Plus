package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Hub runs several workers (this account logged in several times, each with its own
// token) as one opener, keeps them all running, aims them (fixed capsule, next-on-Mystical
// or Mystical hunt) and gathers the numbers the window shows.

var (
	superviseEvery = time.Second
	syncEvery      = 3 * time.Second
	fillGap        = 2 * time.Second
	sampleEvery    = 5 * time.Second
)

type Hub struct {
	leastPick   string // trian the Least / Wish mode is on
	leastTarget string
	mu          sync.Mutex
	primary     *App
	workers     []*App
	pool        *CoinPool

	want      bool // the person pressed Start
	supOn     bool
	filling   bool
	lastSync  time.Time
	lastSamp  time.Time
	startedAt time.Time

	pulls   int
	counts  map[string]int
	series  []int
	perCap  map[string]*capStat
	hunt    huntFile
	huntRow []HuntRow

	aucMsg  string
	aucBusy bool

	speedMsg  string
	speedBusy bool

	sumHour int64 // the clock hour whose summary is due next

	odds      map[string]*capOdds // lifetime pulls per capsule (odds.json)
	etaKey    string              // what the cached ETA was computed from
	etaCache  []ETAGroup
	luckAt    time.Time
	luckCache LuckInfo
	oddsMu    sync.Mutex
	oddsDirty bool
	upd       *UpdateInfo
	updBusy   bool
	updMsg    string
	updNote   string
	canAuto   func() bool               // may an update be installed by itself right now?
	applyUpd  func(newExe string) error // swaps the program and restarts (set by main)
	updTried  time.Time
	bootAt    time.Time

	hist      []HistBucket
	histMu    sync.Mutex
	histDirty bool
	lastHist  time.Time

	log       []PullEvent
	logMu     sync.Mutex
	mystRun   int  // Mysticals pulled since Start
	lowWarned bool // low-coins alert already shown
	outWarned bool // out-of-coins alert already shown

	quit chan struct{}
	wg   sync.WaitGroup // background goroutines, so Close can wait for them
}

type capStat struct{ Pulls, Mystical int }

// huntFile is kept between runs so "packs per Mystical" gets more accurate over time.
type huntFile struct {
	Pulls    int `json:"pulls"`
	Mystical int `json:"mystical"`
	Since    int `json:"since"`
}

func NewHub(dir string) *Hub {
	h := &Hub{
		primary: NewApp(dir),
		pool:    newCoinPool(),
		counts:  map[string]int{},
		perCap:  map[string]*capStat{},
		quit:    make(chan struct{}),
	}
	p := h.primary
	p.pool = h.pool
	p.onPull = h.onPull
	p.slots = 1
	h.workers = []*App{p}
	if b, err := os.ReadFile(filepath.Join(dir, "hunt.json")); err == nil {
		_ = json.Unmarshal(b, &h.hunt)
	}
	h.loadHistory()
	h.loadOdds()
	h.bootAt = time.Now()
	return h
}

func (h *Hub) saveHunt() {
	h.mu.Lock()
	b, _ := json.Marshal(h.hunt)
	h.mu.Unlock()
	_ = os.WriteFile(filepath.Join(h.primary.dir, "hunt.json"), b, 0o600)
}

// newExtra makes another worker for the same account.
func (h *Hub) newExtra() *App {
	p := h.primary
	p.mu.Lock()
	cfg := p.cfg
	chances, hints := p.chances, p.hints
	caps := append([]string(nil), p.capsules...)
	p.mu.Unlock()
	a := &App{
		dir:        p.dir,
		cfg:        cfg,
		client:     &http.Client{Transport: newTransport(), Timeout: p.client.Timeout}, // its own connection(s) to the site
		errBackoff: p.errBackoff,
		lastNew:    "NONE",
		lastRare:   "NONE",
		tally:      map[string]map[string]int{},
		chances:    chances,
		hints:      hints,
		capsules:   caps,
		noSave:     true,
		pool:       h.pool,
		onPull:     h.onPull,
	}
	return a
}

// ---------- settings ----------

func (h *Hub) cfg() Config {
	h.primary.mu.Lock()
	defer h.primary.mu.Unlock()
	return h.primary.cfg
}

func (h *Hub) update(f func(c *Config)) {
	p := h.primary
	p.mu.Lock()
	f(&p.cfg)
	p.cfg.fix()
	p.saveConfigLocked()
	p.mu.Unlock()
	h.sync()
}

func (h *Hub) SetDelay(ms float64)    { h.update(func(c *Config) { c.DelayMs = maxf(ms, 0) }) }
func (h *Hub) SetInFlight(n int)      { h.update(func(c *Config) { c.InFlight = n }) }
func (h *Hub) SetWorkers(n int)       { h.update(func(c *Config) { c.Workers = n }) }
func (h *Hub) SetMode(m string)       { h.update(func(c *Config) { c.Mode = m }) }
func (h *Hub) SetSpread(n int)        { h.update(func(c *Config) { c.HuntSpread = n }) }
func (h *Hub) SelectCapsule(n string) { h.update(func(c *Config) { c.Capsule = n }) }
func (h *Hub) SetGoal(n int) {
	h.update(func(c *Config) { c.StopAfterMystical = maxi(0, mini(n, 999)) })
}
func (h *Hub) SetChase(on bool) { h.update(func(c *Config) { c.HuntMissing = on }) }
func (h *Hub) SetSound(on bool) { h.update(func(c *Config) { c.AlertSound = on }) }

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// sync copies the shared settings to every worker and aims each one at a capsule.
func (h *Hub) sync() {
	p := h.primary
	p.mu.Lock()
	cfg := p.cfg
	chances, hints := p.chances, p.hints
	caps := append([]string(nil), p.capsules...)
	p.mu.Unlock()

	p.mu.Lock()
	inv := p.inv
	p.mu.Unlock()
	var owned map[string]bool
	if len(inv) > 0 && cfg.HuntMissing {
		_, _, owned = collection(inv)
	}
	var measured map[string][2]int
	if cfg.HuntMeasured {
		measured = h.measuredMystical()
	}
	rows := huntRankingM(caps, chances, owned, measured)

	h.mu.Lock()
	n := len(h.workers)
	targets := make([]string, n)
	for i := range targets {
		targets[i] = cfg.Capsule
	}
	target := ""
	switch cfg.Mode {
	case "hunt":
		targets = huntAssign(rows, cfg.HuntSpread, n)
	case "least", "wish":
		var only []string
		if cfg.Mode == "wish" {
			only = append([]string{}, cfg.Wishlist...)
		}
		name, capsule := leastTarget(inv, only, h.leastPick)
		h.leastPick = name
		if capsule != "" && len(inv) > 0 {
			target = name
			for i := range targets {
				targets[i] = capsule
			}
		}
	}
	h.leastTarget = target
	for i := range rows {
		for _, t := range targets {
			if t == rows[i].Capsule {
				rows[i].Workers++
			}
		}
	}
	h.huntRow = rows
	ws := append([]*App(nil), h.workers...)
	h.mu.Unlock()

	for i, w := range ws {
		w.mu.Lock()
		w.slot, w.slots = i, n
		if w != p {
			w.cfg.BaseURL, w.cfg.DelayMs, w.cfg.InFlight = cfg.BaseURL, cfg.DelayMs, cfg.InFlight
			w.cfg.MaxErrorsInRow, w.cfg.CoinCost = cfg.MaxErrorsInRow, cfg.CoinCost
			w.cfg.Username, w.cfg.Password = cfg.Username, cfg.Password
			w.cfg.LoginPath, w.cfg.LoginField = cfg.LoginPath, cfg.LoginField
			w.chances, w.hints, w.capsules = chances, hints, caps
		}
		if (cfg.Mode == "hunt" || (target != "" && (cfg.Mode == "least" || cfg.Mode == "wish"))) && targets[i] != "" {
			w.cfg.Capsule = targets[i]
		} else if w != p {
			w.cfg.Capsule = cfg.Capsule
		}
		w.mu.Unlock()
	}
}

// ---------- start / stop ----------

func (h *Hub) Start() {
	h.mu.Lock()
	h.want = true
	if h.startedAt.IsZero() {
		h.startedAt = time.Now()
		h.mystRun = 0
	}
	if !h.supOn {
		h.supOn = true
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.supervise()
		}()
	}
	h.mu.Unlock()
	h.sync()
	cfg := h.cfg()
	h.bg(func() { h.primary.Warm(cfg.Workers * cfg.InFlight) })
}

func (h *Hub) Stop() {
	h.mu.Lock()
	h.want = false
	h.startedAt = time.Time{}
	ws := append([]*App(nil), h.workers...)
	h.mu.Unlock()
	for _, w := range ws {
		w.Stop()
	}
}

func (h *Hub) wanted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.want
}

// supervise keeps every worker running while the person wants opening.
func (h *Hub) supervise() {
	for {
		select {
		case <-h.quit:
			return
		case <-time.After(superviseEvery):
		}
		h.tick()
	}
}

// Close stops everything and waits for the background work to finish.
func (h *Hub) Close() {
	h.Stop()
	h.mu.Lock()
	ws := append([]*App(nil), h.workers...)
	h.mu.Unlock()
	for _, w := range ws {
		w.loops.Wait()
	}
	select {
	case <-h.quit:
	default:
		close(h.quit)
	}
	h.wg.Wait()
	h.flushHistory()
	h.flushOdds()
}

// bg runs f in the background; Close waits for it.
func (h *Hub) bg(f func()) {
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		f()
	}()
}

func (h *Hub) tick() {
	h.sync()
	h.mu.Lock()
	want := h.want
	h.mu.Unlock()

	// the main account's login, capsule list, inventory
	p := h.primary
	if want {
		h.fillWorkers()
	}
	if time.Since(h.lastSync) >= syncEvery {
		h.lastSync = time.Now()
		h.bg(func() {
			if p.hasToken() {
				if err := p.SyncInventory(); err != nil {
					p.logf("inventory sync: %v", err)
				}
			}
		})
	}
	h.maybeCheckUpdate()
	h.maybeSummary()
	if time.Since(h.lastHist) >= 20*time.Second {
		h.lastHist = time.Now()
		h.recordCoins(h.pool.get())
		h.flushHistory()
		h.flushOdds()
	}
	if time.Since(h.lastSamp) >= sampleEvery {
		h.lastSamp = time.Now()
		h.mu.Lock()
		h.series = append(h.series, h.pulls)
		if len(h.series) > 360 {
			h.series = h.series[len(h.series)-360:]
		}
		h.mu.Unlock()
	}
	if !want {
		return
	}
	h.mu.Lock()
	ws := append([]*App(nil), h.workers...)
	h.mu.Unlock()
	cost := int64(h.cfg().CoinCost)
	for _, w := range ws {
		h.ensureRunning(w, cost)
	}
	h.checkAlerts()
}

func (a *App) hasToken() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Token != ""
}

func (a *App) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// ensureRunning (re)starts a worker that has stopped, unless there is a reason it must stay stopped.
func (h *Hub) ensureRunning(w *App, cost int64) {
	w.mu.Lock()
	if w.running || w.logging {
		w.mu.Unlock()
		return
	}
	if w.outCoins {
		if c := h.pool.get(); c >= 0 && c < cost {
			w.mu.Unlock()
			return // still out of coins
		}
		w.outCoins = false
	}
	if w.wrongLogin {
		w.mu.Unlock()
		return
	}
	needToken := w.cfg.Token == "" || (!w.exp.IsZero() && time.Until(w.exp) < 45*time.Second)
	hasCreds := w.hasCredsLocked()
	capsule := w.cfg.Capsule
	if needToken && hasCreds {
		w.logging = true
		w.mu.Unlock()
		go func() {
			err := w.LoginRetry(func() bool { return !h.wanted() })
			w.mu.Lock()
			w.logging = false
			if err != nil && strings.Contains(err.Error(), "rejected") {
				w.wrongLogin = true
				defer h.alert("warn", "Login rejected", "Wrong username or password. Open the window and log in again.")
			}
			w.mu.Unlock()
			if err == nil && w == h.primary {
				_ = w.LoadCapsules()
				_ = w.SyncInventory()
			}
		}()
		return
	}
	w.mu.Unlock()
	if capsule == "" {
		return
	}
	w.Start()
}

// fillWorkers adds extra workers up to the configured number, one every fillGap.
func (h *Hub) fillWorkers() {
	h.mu.Lock()
	if h.filling {
		h.mu.Unlock()
		return
	}
	target := 1
	h.mu.Unlock()
	target = h.cfg().Workers
	h.mu.Lock()
	// remove surplus workers
	for len(h.workers) > target && len(h.workers) > 1 {
		w := h.workers[len(h.workers)-1]
		h.workers = h.workers[:len(h.workers)-1]
		go w.Stop()
	}
	if len(h.workers) >= target {
		h.mu.Unlock()
		return
	}
	h.filling = true
	h.mu.Unlock()
	go func() {
		for {
			h.mu.Lock()
			more := len(h.workers) < h.cfg().Workers && h.want
			h.mu.Unlock()
			if !more {
				break
			}
			w := h.newExtra()
			h.mu.Lock()
			h.workers = append(h.workers, w)
			h.mu.Unlock()
			h.sync()
			time.Sleep(fillGap)
		}
		h.mu.Lock()
		h.filling = false
		h.mu.Unlock()
	}()
}

// ---------- pull hook ----------

func (h *Hub) onPull(a *App, capsule string, r openResp) {
	p := h.primary
	if r.Rarity == "Chroma" || r.Rarity == "Mystical" {
		p.considerRare(capsule, r.Trian, r.Rarity)
	}
	h.mu.Lock()
	h.pulls++
	h.counts[r.Rarity]++
	cs := h.perCap[capsule]
	if cs == nil {
		cs = &capStat{}
		h.perCap[capsule] = cs
	}
	cs.Pulls++
	h.hunt.Pulls++
	h.hunt.Since++
	isMyst := r.Rarity == "Mystical"
	if isMyst {
		cs.Mystical++
		h.hunt.Mystical++
		h.hunt.Since = 0
	}
	h.mu.Unlock()
	h.recordHist(r.Rarity)
	h.recordOdds(capsule, r.Rarity)
	h.logPull(capsule, r)
	if r.New && h.wished(r.Trian) {
		h.alert("rare", "Wishlist trian!", fmt.Sprintf("%s from the %s capsule", r.Trian, capsule))
	}
	if r.Rarity == "Chroma" || isMyst {
		p.mu.Lock()
		p.lastRare = r.Trian
		p.mu.Unlock()
		h.alert("rare", r.Rarity+" pulled!", fmt.Sprintf("%s from the %s capsule", r.Trian, capsule))
		h.bg(func() { h.webhook(r, capsule) })
	}
	if isMyst {
		h.mu.Lock()
		h.mystRun++
		reached := h.cfg().StopAfterMystical > 0 && h.mystRun >= h.cfg().StopAfterMystical && h.want
		h.mu.Unlock()
		if reached {
			h.bg(func() {
				h.Stop()
				h.alert("rare", "Goal reached", fmt.Sprintf("%d Mystical pulled - opening stopped.", h.cfg().StopAfterMystical))
			})
		}
		h.advanceAfterMystical(capsule)
		h.bg(h.saveHunt)
	}
}

// advanceAfterMystical moves the main capsule on to the next one in the order.
func (h *Hub) advanceAfterMystical(from string) {
	p := h.primary
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cfg.Mode != "advance" || p.cfg.NoAdvance || p.cfg.Capsule != from {
		return
	}
	order := p.capsules
	if len(order) == 0 {
		order = builtinCapsules
	}
	for i, c := range order {
		if c == from {
			p.cfg.Capsule = order[(i+1)%len(order)]
			p.saveConfigLocked()
			return
		}
	}
}

// ResetStats clears the session numbers.
func (h *Hub) ResetStats() {
	h.mu.Lock()
	h.pulls = 0
	h.counts = map[string]int{}
	h.series = nil
	h.perCap = map[string]*capStat{}
	h.hunt = huntFile{}
	h.mu.Unlock()
	h.primary.ResetStats()
	h.saveHunt()
}

// ---------- snapshot ----------

func (h *Hub) Snapshot() Snapshot {
	p := h.primary
	s := p.Snapshot()
	cfg := h.cfg()
	h.mu.Lock()
	ws := append([]*App(nil), h.workers...)
	s.Opened = h.pulls
	s.Counts = map[string]int{}
	for k, v := range h.counts {
		s.Counts[k] = v
	}
	s.Series = append([]int(nil), h.series...)
	s.Hunt = append([]HuntRow(nil), h.huntRow...)
	s.Mystical = h.hunt.Mystical
	s.SinceMystical = h.hunt.Since
	if h.hunt.Mystical > 0 {
		s.PacksPerMystical = float64(h.hunt.Pulls) / float64(h.hunt.Mystical)
		s.CoinsPerMystical = s.PacksPerMystical * float64(cfg.CoinCost)
	}
	s.AuctionMsg, s.AuctionBusy = h.aucMsg, h.aucBusy
	s.SpeedMsg, s.SpeedBusy = h.speedMsg, h.speedBusy
	s.Hist = h.historySnapshot()
	if h.upd != nil {
		s.UpdateVer, s.UpdateNotes = h.upd.Version, h.upd.Notes
	}
	s.UpdateBusy, s.UpdateMsg, s.UpdateNote = h.updBusy, h.updMsg, h.updNote
	s.AutoUpdate = cfg.AutoUpdate
	s.Theme, s.NtfyOn, s.NtfyTopic, s.NoUpdateCheck = normTheme(cfg.Theme), cfg.NtfyOn, cfg.NtfyTopic, cfg.NoUpdateCheck
	s.Protected = sealImpl != nil
	s.KeepAwake = cfg.KeepAwake
	s.HourlySummary = cfg.HourlySummary
	s.Wishlist = append([]string(nil), cfg.Wishlist...)
	s.Target = h.leastTarget // h.mu is held here
	s.DownText = FormatDowntime(cfg.Downtime)
	s.Down, s.DownEnd = DownNow(cfg.Downtime, time.Now())
	if cfg.WhatsNewVer == appVersion {
		s.WhatsNew = cfg.WhatsNewNotes
		if s.WhatsNew == "" {
			s.WhatsNew = "Updated to version " + appVersion + "."
		}
	}
	list, idx := h.Accounts()
	for _, a := range list {
		s.AcctNames = append(s.AcctNames, a.Name)
	}
	s.AcctIdx = idx
	if h.want && !h.startedAt.IsZero() {
		s.StartedAgo = int(time.Since(h.startedAt).Seconds())
	}
	want := h.want
	h.mu.Unlock()

	s.Mode, s.HuntSpread, s.Workers, s.InFlight, s.CoinCost = cfg.Mode, cfg.HuntSpread, cfg.Workers, cfg.InFlight, cfg.CoinCost
	run := 0
	for _, w := range ws {
		if w.isRunning() {
			run++
		}
	}
	s.WorkersRunning = run
	s.Running = want || run > 0
	if len(s.Series) >= 2 {
		n := len(s.Series)
		span := 12
		if n-1 < span {
			span = n - 1
		}
		s.PerMin = float64(s.Series[n-1]-s.Series[n-1-span]) / (float64(span) * sampleEvery.Seconds()) * 60
	}

	p.mu.Lock()
	s.HasAcct, s.Coins, s.AcctOpened = p.hasAcct, p.acctCoins, p.acctOpened
	s.Inv = append([]InvItem(nil), p.inv...)
	if c := h.pool.get(); c >= 0 {
		s.Coins = int(c)
		if p.hasAcct && c == 0 && p.acctCoins > 0 {
			s.Coins = p.acctCoins
		}
	}
	p.mu.Unlock()
	if s.HasAcct && cfg.CoinCost > 0 {
		s.PacksLeft = s.Coins / cfg.CoinCost
	}
	for _, it := range s.Inv {
		s.InvTotal += it.Qty
	}
	s.Log = h.recentLog(80)
	s.TotalTrians = len(builtinTrians)
	if len(s.Inv) > 0 {
		s.Collected, s.Missing, _ = collection(s.Inv)
	}
	h.mu.Lock()
	s.StopAfter, s.MysticalRun = cfg.StopAfterMystical, h.mystRun
	h.mu.Unlock()
	s.AlertSound, s.HuntMissing = cfg.AlertSound, cfg.HuntMissing
	s.Luck, s.ETA = h.luckAndETA(s, cfg.CoinCost)
	return s
}

// SetDowntime stores the pause windows typed as text ("3:20pm-4pm, 2am-3am"). Empty clears them.
func (h *Hub) SetDowntime(text string) string {
	w, err := ParseDowntime(text)
	if err != nil {
		return err.Error()
	}
	h.update(func(c *Config) { c.Downtime = w })
	return ""
}

func (h *Hub) wished(name string) bool {
	k := lowerKey(name)
	for _, w := range h.cfg().Wishlist {
		if lowerKey(w) == k {
			return true
		}
	}
	return false
}

// ToggleWish adds the trian to the wishlist, or removes it if it is already there.
func (h *Hub) ToggleWish(name string) {
	h.update(func(c *Config) {
		k := lowerKey(name)
		for i, w := range c.Wishlist {
			if lowerKey(w) == k {
				c.Wishlist = append(append([]string{}, c.Wishlist[:i]...), c.Wishlist[i+1:]...)
				return
			}
		}
		for _, t := range builtinTrians {
			if lowerKey(t.Name) == k {
				c.Wishlist = append(append([]string{}, c.Wishlist...), t.Name)
				return
			}
		}
	})
}
