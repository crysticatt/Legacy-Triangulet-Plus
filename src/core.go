package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	appName     = "TrianguletOpener"
	defaultBase = "https://tri.pengpowers.xyz"
)

// Config is stored in %APPDATA%\TrianguletOpener\config.json
type Config struct {
	BaseURL        string  `json:"baseUrl"`
	Token          string  `json:"token"`
	Capsule        string  `json:"capsule"`
	DelayMs        float64 `json:"delayMs"` // wait between pulls in milliseconds (down to 0.000001)
	MaxErrorsInRow int     `json:"maxErrorsInRow"`
	AutoStart      bool    `json:"autoStart"`
	Username       string  `json:"username"`
	Password       string  `json:"password"`
	StartupInit    bool    `json:"startupInit"` // run-at-startup was switched on once by default
	LoginPath      string  `json:"loginPath"`   // remembered after the first successful login
	LoginField     string  `json:"loginField"`  // "name" or "username"

	InFlight   int    `json:"inFlight"`   // requests running at once per worker
	Workers    int    `json:"workers"`    // total workers (this account logged in several times)
	Mode       string `json:"mode"`       // "fixed", "advance" (next capsule after a Mystical) or "hunt"
	HuntSpread int    `json:"huntSpread"` // hunt: how many of the best capsules to split the workers across
	NoAdvance  bool   `json:"noAdvance"`
	DiscordOn  bool   `json:"discordOn"`
	DiscordID  string `json:"discordId"`
	WebhookURL string `json:"webhookUrl"`
	CoinCost   int    `json:"coinCost"` // coins per capsule (25)

	HuntMissing       bool      `json:"huntMissing"`       // hunt: aim at Mystical trians you don't own yet (falls back to all Mysticals)
	StopAfterMystical int       `json:"stopAfterMystical"` // stop opening after this many Mysticals in one run (0 = never)
	AlertSound        bool      `json:"alertSound"`
	Accounts          []Account `json:"accounts"`      // saved logins to switch between
	UpdateRepo        string    `json:"updateRepo"`    // GitHub owner/repo to check for new versions (empty = default)
	NoUpdateCheck     bool      `json:"noUpdateCheck"` // never look for updates
	CfgVer            int       `json:"cfgVer"`        // settings file generation, for one-time upgrades
	KeepAwake         bool      `json:"keepAwake"`     // stop the PC sleeping while opening (the screen may still turn off)
	Theme             string    `json:"theme"`         // dark, light or blue
	NtfyOn            bool      `json:"ntfyOn"`        // send alerts to your phone through ntfy
	NtfyTopic         string    `json:"ntfyTopic"`     // your private ntfy topic name
	NtfyServer        string    `json:"ntfyServer"`    // empty = https://ntfy.sh
	AutoUpdate        bool      `json:"autoUpdate"`    // install updates by itself (Windows: only while the window is hidden)
	WhatsNewVer       string    `json:"whatsNewVer"`   // set when an update was installed: show its notes once
	WhatsNewNotes     string    `json:"whatsNewNotes"`
	AlertLowPacks     int       `json:"alertLowPacks"` // warn when fewer than this many packs are left (0 = off)
}

type openResp struct {
	Rarity string `json:"rarity"`
	Trian  string `json:"trian"`
	New    bool   `json:"new"`
}

type authError struct{ code int }

func (e authError) Error() string {
	return fmt.Sprintf("token rejected as unauthorized (HTTP %d)", e.code)
}

type rateError struct{ wait time.Duration }

func (e rateError) Error() string { return "rate limited" }

var errNoToken = errors.New("no token set")

// App holds all opener state. Safe for use from the UI thread and the open loop.
type App struct {
	mu  sync.Mutex
	dir string
	cfg Config

	client *http.Client

	running  bool
	gen      int
	resume   bool // restart automatically after a new token is set
	capsules []string
	opened   int
	lastNew  string
	lastRare string
	notice   string
	tally    map[string]map[string]int
	tokenIdx int

	expiryWarned bool
	bal          []balloon // queued tray notifications

	errBackoff   time.Duration // first wait after a failed open (doubles each time)
	exp          time.Time     // expiry of the current token (zero if unknown)
	lastLoginTry time.Time
	info         string // last login message shown in the window

	chances map[string]float64 // drop chance (percent) per capsule/trian, from the capsule list
	hints   map[string]string  // image paths, if the capsule list provides them
	best    *bestChroma        // rarest Chroma ever pulled (kept across runs)
	bestImg *image.RGBA        // its picture (64x64) or a placeholder
	iconVer int                // bumps whenever bestImg changes

	// set by the Hub (nil when the App runs on its own)
	pool       *CoinPool
	onPull     func(a *App, capsule string, r openResp)
	slot       int  // this worker's place in the shared stagger clock
	slots      int  // total workers in that clock
	noSave     bool // extra workers keep their settings in memory only
	outCoins   bool // stopped because the account has fewer coins than a capsule costs
	rarOf      map[string]string
	started    time.Time
	loops      sync.WaitGroup // running loop goroutines, so a Hub can wait for them
	logging    bool           // a login is in progress for this worker
	wrongLogin bool           // the site rejected the username/password

	inv        []InvItem
	acctCoins  int
	acctOpened int
	hasAcct    bool
}

func dataDir() string {
	if d := os.Getenv("TRIOPENER_DIR"); d != "" {
		_ = os.MkdirAll(d, 0o700)
		return d
	}
	base, err := os.UserConfigDir() // %AppData% on Windows
	if err != nil || base == "" {
		base = "."
	}
	d := filepath.Join(base, appName)
	_ = os.MkdirAll(d, 0o700)
	return d
}

func NewApp(dir string) *App {
	a := &App{
		errBackoff: 250 * time.Millisecond,
		dir:        dir,
		lastNew:    "NONE",
		lastRare:   "NONE",
		tally:      map[string]map[string]int{},
		cfg: Config{
			BaseURL:        defaultBase,
			DelayMs:        2,
			MaxErrorsInRow: 25,
			InFlight:       2,
			Workers:        1,
			Mode:           "advance",
			HuntSpread:     1,
			AutoStart:      true,
			DiscordOn:      true,
			DiscordID:      defaultDiscordID,
			CoinCost:       25,
			HuntMissing:    true,
			KeepAwake:      true,
			AutoUpdate:     true,
			AlertSound:     true,
			AlertLowPacks:  20,
		},
	}
	a.client = &http.Client{Transport: sharedTransport(), Timeout: 20 * time.Second}

	path := filepath.Join(dir, "config.json")
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &a.cfg) // missing keys keep defaults
		if a.cfg.CfgVer < 38 {        // v38: installing updates by itself is now on by default
			a.cfg.AutoUpdate, a.cfg.CfgVer = true, 38
			defer a.saveConfigLocked()
		}
		if openConfig(&a.cfg) {
			a.logf("some saved logins could not be unlocked (config.json came from another PC or user?). Log in again.")
		}
		if sealImpl != nil && hasPlainSecret(string(b)) {
			a.saveConfigLocked() // protect a password that an older version saved as plain text
		}
	} else {
		a.saveConfigLocked()
	}
	a.cfg.BaseURL = strings.TrimRight(a.cfg.BaseURL, "/")
	if a.cfg.BaseURL == "" {
		a.cfg.BaseURL = defaultBase
	}
	if a.cfg.DelayMs < 0 {
		a.cfg.DelayMs = 0
	}
	a.cfg.fix()
	if a.cfg.MaxErrorsInRow < 1 {
		a.cfg.MaxErrorsInRow = 25
	}
	_, a.exp, _ = tokenInfo(a.cfg.Token)
	a.chances = map[string]float64{}
	a.hints = map[string]string{}
	a.loadBest()
	return a
}

func (a *App) saveConfigLocked() {
	if a.noSave {
		return
	}
	b, err := json.MarshalIndent(sealConfig(a.cfg), "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(a.dir, "config.json"), b, 0o600)
}

func (a *App) logf(format string, args ...any) {
	p := filepath.Join(a.dir, "opener.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 200_000 {
		_ = os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

// ---------- token handling ----------

var jwtRe = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

// cleanToken extracts a usable token from whatever was copied.
func cleanToken(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "'\"` \r\n\t")
	if len(s) > 4096 {
		return "", errors.New("clipboard text is too long to be a token")
	}
	m := jwtRe.FindString(s)
	if m == "" {
		return "", errors.New("no token found in the clipboard")
	}
	// Keep a short prefix such as "triangulet " if the site's value includes it.
	if strings.HasSuffix(s, m) && len(s)-len(m) <= 32 {
		return s, nil
	}
	return m, nil
}

// tokenCandidates lists the authorization header values to try, in order.
func tokenCandidates(raw string) []string {
	if raw == "" {
		return nil
	}
	out := []string{raw}
	add := func(v string) {
		for _, e := range out {
			if e == v {
				return
			}
		}
		out = append(out, v)
	}
	if j := jwtRe.FindString(raw); j != "" {
		add(j)
		add("triangulet " + j)
	}
	return out
}

func tokenInfo(raw string) (name string, exp time.Time, ok bool) {
	j := jwtRe.FindString(raw)
	if j == "" {
		return "", time.Time{}, false
	}
	parts := strings.Split(j, ".")
	if len(parts) < 2 {
		return "", time.Time{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", time.Time{}, false
	}
	var p struct {
		Name string `json:"name"`
		Exp  int64  `json:"exp"`
	}
	if json.Unmarshal(b, &p) != nil {
		return "", time.Time{}, false
	}
	if p.Exp > 0 {
		exp = time.Unix(p.Exp, 0)
	}
	return p.Name, exp, true
}

// SetToken stores a token from copied text and returns a short description.
func (a *App) SetToken(text string) (string, error) {
	tok, err := cleanToken(text)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.setTokenLocked(tok)
	a.notice = ""
	desc := a.tokenDescLocked()
	resume := a.resume
	a.resume = false
	a.mu.Unlock()
	if resume {
		a.Start()
	}
	return desc, nil
}

func (a *App) tokenDescLocked() string {
	if a.cfg.Token == "" {
		return ""
	}
	name, exp, ok := tokenInfo(a.cfg.Token)
	if !ok {
		return "set"
	}
	desc := name
	if desc == "" {
		desc = "token"
	}
	if !exp.IsZero() {
		if time.Now().After(exp) {
			desc += ", expired " + exp.Local().Format("3:04 PM")
		} else {
			desc += ", expires " + exp.Local().Format("3:04 PM")
		}
	}
	return desc
}

// CheckExpiry queues a one-time heads-up shortly before the token expires.
func (a *App) CheckExpiry() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.Token == "" || a.expiryWarned || a.hasCredsLocked() {
		return // with a saved login the token is renewed automatically
	}
	_, exp, ok := tokenInfo(a.cfg.Token)
	if !ok || exp.IsZero() {
		return
	}
	if left := time.Until(exp); left > 0 && left < 5*time.Minute {
		a.expiryWarned = true
		a.bal = append(a.bal, balloon{"Token expires soon", "Copy a fresh token from the site and choose Set token from clipboard."})
	}
}

type balloon struct{ title, text string }

func (a *App) pushBalloon(title, text string) {
	a.mu.Lock()
	if len(a.bal) < 20 {
		a.bal = append(a.bal, balloon{title, text})
	}
	a.mu.Unlock()
}

// takeBalloon pops the oldest queued notification (the tray shows one per tick).
func (a *App) takeBalloon() (title, text string, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.bal) == 0 {
		return "", "", false
	}
	b := a.bal[0]
	a.bal = a.bal[1:]
	return b.title, b.text, true
}

// ---------- HTTP ----------

func (a *App) do(method, path string, body []byte) (*http.Response, error) {
	for {
		a.mu.Lock()
		tok := a.cfg.Token
		cands := tokenCandidates(tok)
		idx := a.tokenIdx
		base := a.cfg.BaseURL
		a.mu.Unlock()
		if len(cands) == 0 {
			return nil, errNoToken
		}
		if idx >= len(cands) {
			idx = len(cands) - 1
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, base+path, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("authorization", cands[idx])
		resp, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		// Read the reply up front: this site can answer a bad token with
		// HTTP 200 and {"message":"unauthorized"}, so the body must be checked too.
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		if isAuthFailure(resp.StatusCode, raw) {
			a.mu.Lock()
			changed := a.cfg.Token != tok // another lane logged in again while this request was out
			if !changed && idx < len(cands)-1 && a.tokenIdx == idx {
				a.tokenIdx++
			}
			a.mu.Unlock()
			if changed || idx < len(cands)-1 {
				continue
			}
		}
		return resp, nil
	}
}

func (a *App) LoadCapsules() error {
	resp, err := a.do("GET", "/data/trians", nil)
	if err != nil {
		a.setNotice("Could not load capsules: " + err.Error())
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if isAuthFailure(resp.StatusCode, raw) {
		a.setNotice("Token rejected or expired")
		a.pushBalloon("Token rejected", "The site said unauthorized. Log in again from the window.")
		return authError{resp.StatusCode}
	}
	if resp.StatusCode != 200 {
		err := fmt.Errorf("HTTP %d%s", resp.StatusCode, snippet(raw))
		a.setNotice("Could not load capsules: " + err.Error())
		return err
	}
	var d struct {
		Capsules []struct {
			Name   string           `json:"name"`
			Trians []map[string]any `json:"trians"`
		} `json:"ValuesnCapsules"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		a.setNotice("Could not read capsule list" + snippet(raw))
		return err
	}
	names := make([]string, 0, len(d.Capsules))
	chances := map[string]float64{}
	hints := map[string]string{}
	for _, c := range d.Capsules {
		names = append(names, c.Name)
		for _, t := range c.Trians {
			tn, _ := t["name"].(string)
			if tn == "" {
				continue
			}
			if f, ok := toFloat(t["chance"]); ok {
				chances[chanceKey(c.Name, tn)] = f
			}
			for _, k := range []string{"img", "image", "icon", "src", "url", "path"} {
				if s, ok := t[k].(string); ok && s != "" {
					hints[chanceKey(c.Name, tn)] = s
					break
				}
			}
		}
	}
	a.mu.Lock()
	a.capsules = names
	a.chances = chances
	a.hints = hints
	if a.cfg.Capsule == "" && len(names) > 0 {
		a.cfg.Capsule = names[0]
		a.saveConfigLocked()
	}
	if strings.HasPrefix(a.notice, "Could not load capsules") {
		a.notice = ""
	}
	a.mu.Unlock()
	return nil
}

func (a *App) openOnce(capsule string) (openResp, error) {
	body, _ := json.Marshal(map[string]string{"capsule": capsule})
	resp, err := a.do("POST", "/api/open", body)
	if err != nil {
		return openResp{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		io.Copy(io.Discard, resp.Body)
		return openResp{}, authError{resp.StatusCode}
	case resp.StatusCode == 429:
		io.Copy(io.Discard, resp.Body)
		return openResp{}, rateError{}
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if isAuthFailure(resp.StatusCode, raw) {
		return openResp{}, authError{resp.StatusCode}
	}
	if resp.StatusCode != 200 {
		return openResp{}, fmt.Errorf("HTTP %d%s", resp.StatusCode, snippet(raw))
	}
	var r openResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return openResp{}, fmt.Errorf("unreadable response%s", snippet(raw))
	}
	if r.Trian == "" || r.Rarity == "" {
		return openResp{}, fmt.Errorf("unexpected response%s", snippet(raw))
	}
	return r, nil
}

// isAuthFailure reports whether a reply means "your token isn't accepted": an HTTP
// 401/403, or a short reply (any status) such as {"message":"unauthorized"}.
func isAuthFailure(status int, body []byte) bool {
	if status == 401 || status == 403 {
		return true
	}
	if len(body) > 400 {
		return false
	}
	s := strings.ToLower(string(body))
	for _, w := range []string{"unauthorized", "unauthenticated", "invalid token", "token expired", "jwt expired", "not logged in", "invalid signature"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// snippet returns ": <first part of the server's reply>" for error messages.
func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if s == "" {
		return ""
	}
	if r := []rune(s); len(r) > 140 {
		s = string(r[:140]) + "..."
	}
	return ": " + s
}

// ---------- open loop ----------

func (a *App) active(gen int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running && a.gen == gen
}

func (a *App) halt(gen int, reason string, resume bool) {
	a.mu.Lock()
	if a.gen == gen {
		a.running = false
		a.gen++
		a.notice = reason
		a.resume = resume
	}
	a.mu.Unlock()
}

func (a *App) record(r openResp, capsule string) {
	if a.onPull != nil {
		defer a.onPull(a, capsule, r)
	} else if r.Rarity == "Chroma" {
		a.considerChroma(capsule, r.Trian)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.opened++
	b := a.tally[r.Rarity]
	if b == nil {
		b = map[string]int{}
		a.tally[r.Rarity] = b
	}
	b[r.Trian]++
	if r.New {
		a.lastNew = r.Trian
	}
	if r.Rarity == "Chroma" || r.Rarity == "Mystical" {
		a.lastRare = r.Trian
	}
	a.notice = ""
}

func (a *App) setNotice(s string) {
	a.mu.Lock()
	a.notice = s
	a.mu.Unlock()
}

func (a *App) Start() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running || a.cfg.Token == "" || a.cfg.Capsule == "" {
		return false
	}
	a.running = true
	a.gen++
	a.notice = ""
	gen := a.gen
	a.loops.Add(1)
	go func() {
		defer a.loops.Done()
		a.loop(gen)
	}()
	return true
}

func (a *App) Stop() {
	a.mu.Lock()
	a.running = false
	a.resume = false
	a.gen++
	a.mu.Unlock()
}

// loop runs InFlight lanes, each one opening capsules back to back. More lanes means
// more requests on the wire at once, which is what makes opening faster.
func (a *App) loop(gen int) {
	a.mu.Lock()
	lanes := a.cfg.InFlight
	a.started = time.Now()
	a.mu.Unlock()
	if lanes < 1 {
		lanes = 1
	}
	var fails int64
	var wg sync.WaitGroup
	for j := 0; j < lanes; j++ {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			a.lane(gen, j, lanes, &fails)
		}(j)
	}
	wg.Wait()
}

func (a *App) lane(gen, lane, lanes int, fails *int64) {
	var credit int64 // capsules this lane has already paid for
	defer func() {
		a.mu.Lock()
		pool, cost := a.pool, int64(a.cfg.CoinCost)
		a.mu.Unlock()
		if pool != nil {
			pool.returnLease(cost, credit)
		}
	}()
	for a.active(gen) {
		a.mu.Lock()
		capsule := a.cfg.Capsule
		delay := delayDur(a.cfg.DelayMs)
		maxFails := int64(a.cfg.MaxErrorsInRow)
		cost := int64(a.cfg.CoinCost)
		slot, slots, t0 := a.slot, a.slots, a.started
		pool := a.pool
		a.mu.Unlock()
		if slots < 1 {
			slots = 1
		}

		if a.needRefresh() {
			if err := a.Login(); err != nil {
				a.logf("refresh login failed: %v", err)
			}
		}
		if pool != nil && credit == 0 {
			credit = pool.takeLease(cost)
			if credit == 0 {
				if pool.othersHold() { // another lane still holds coins: wait for it to use or return them
					time.Sleep(time.Millisecond)
					continue
				}
			}
		} else if pool != nil {
			pool.useLease(cost)
		}
		if pool != nil && credit == 0 {
			a.halt(gen, "Out of coins", false)
			a.mu.Lock()
			a.outCoins = true
			a.mu.Unlock()
			return
		}
		if credit > 0 {
			credit--
		}
		r, err := a.openOnce(capsule)
		if pool != nil {
			pool.done(cost, err != nil)
		}
		if err == nil {
			atomic.StoreInt64(fails, 0)
			a.record(r, capsule)
		} else {
			var ae authError
			var re rateError
			switch {
			case errors.As(err, &ae):
				a.logf("auth error: %v", err)
				if a.canRelogin() {
					lerr := a.Login()
					if lerr == nil {
						continue
					}
					a.halt(gen, "Auto-login failed: "+lerr.Error(), true)
					a.pushBalloon("Opener stopped: login failed", lerr.Error())
					return
				}
				if a.hasCreds() { // another lane is already logging in: give it a moment
					time.Sleep(20 * time.Millisecond)
					continue
				}
				a.halt(gen, "Token rejected or expired", true)
				a.pushBalloon("Opener stopped: token rejected", "Copy a fresh token from the site and choose Set token from clipboard. It will resume automatically.")
				return
			case errors.Is(err, errNoToken):
				a.halt(gen, "No token set", false)
				return
			case errors.As(err, &re):
				// No waiting: a "slow down" reply just means try again right away.
				runtime.Gosched()
				continue
			case isOutOfCoins(err):
				a.halt(gen, "Out of coins", false)
				a.mu.Lock()
				a.outCoins = true
				a.mu.Unlock()
				return
			}
			n := atomic.AddInt64(fails, 1)
			a.logf("open error (%d in a row): %v", n, err)
			a.setNotice(err.Error())
			if n >= maxFails {
				a.halt(gen, "Stopped after too many errors: "+err.Error(), false)
				a.pushBalloon("Opener stopped", "Too many errors in a row: "+err.Error())
				return
			}
			// Back off (0.25s, 0.5s, 1s ... up to 5s) so a hiccup or a busy server
			// can recover instead of burning through the error limit in a moment.
			wait := a.errBackoff
			for i := int64(1); i < n && wait < 5*time.Second; i++ {
				wait *= 2
			}
			if wait > 5*time.Second {
				wait = 5 * time.Second
			}
			time.Sleep(wait)
			continue
		}
		slotWait(t0, slot*lanes+lane, slots*lanes, delay)
	}
}

func (a *App) hasCreds() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.hasCredsLocked()
}

func isOutOfCoins(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "enough") && (strings.Contains(s, "token") || strings.Contains(s, "coin"))
}

// ---------- UI-facing helpers ----------

type Snapshot struct {
	Running   bool
	Capsule   string
	Capsules  []string
	Opened    int
	LastNew   string
	LastRare  string
	Notice    string
	HasToken  bool
	TokenDesc string
	AutoStart bool
	HasCreds  bool
	Username  string
	Password  string
	DelayMs   float64
	Tally     string
	Info      string

	Counts        map[string]int // opened so far, per rarity
	ChromaName    string         // rarest Chroma pulled (empty if none yet)
	ChromaCapsule string
	ChromaChance  float64 // drop chance in percent (100 = unknown)
	IconVer       int

	// filled in by the Hub
	Mode             string
	HuntSpread       int
	Workers          int // configured
	WorkersRunning   int
	InFlight         int
	CoinCost         int
	PerMin           float64
	Series           []int // pulls so far, sampled every few seconds
	HasAcct          bool
	Coins            int
	AcctOpened       int
	PacksLeft        int
	Mystical         int // Mystical pulled (measured)
	SinceMystical    int
	PacksPerMystical float64 // 0 = not enough data yet
	CoinsPerMystical float64
	Hunt             []HuntRow
	Inv              []InvItem
	InvTotal         int
	AuctionMsg       string
	AuctionBusy      bool
	StartedAgo       int // seconds since opening began (0 if idle)
	Log              []PullEvent
	Collected        int // trians owned (of TotalTrians)
	TotalTrians      int
	Missing          []InvItem // trians not owned yet (Qty 0)
	StopAfter        int
	MysticalRun      int // Mysticals pulled in this run
	AlertSound       bool
	HuntMissing      bool
	AcctNames        []string // saved logins
	UpdateVer        string   // newer version available (empty = none)
	UpdateNotes      string
	UpdateBusy       bool
	UpdateMsg        string
	UpdateNote       string // result of a manual "check for updates"
	AutoUpdate       bool
	Theme            string
	KeepAwake        bool
	NtfyOn           bool
	NtfyTopic        string
	NoUpdateCheck    bool
	Protected        bool   // saved passwords are encrypted on this PC
	WhatsNew         string // release notes of the version just installed (empty = nothing to show)
	AcctIdx          int    // which one is active (-1 = not saved)
	Hist             []HistBucket
	SpeedMsg         string
	SpeedBusy        bool
}

func (a *App) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	counts := map[string]int{}
	for r, items := range a.tally {
		for _, n := range items {
			counts[r] += n
		}
	}
	s := Snapshot{
		Counts:    counts,
		IconVer:   a.iconVer,
		Running:   a.running,
		Capsule:   a.cfg.Capsule,
		Capsules:  append([]string(nil), a.capsules...),
		Opened:    a.opened,
		LastNew:   a.lastNew,
		LastRare:  a.lastRare,
		Notice:    a.notice,
		HasToken:  a.cfg.Token != "",
		TokenDesc: a.tokenDescLocked(),
		AutoStart: a.cfg.AutoStart,
		HasCreds:  a.hasCredsLocked(),
		Username:  a.cfg.Username,
		Password:  a.cfg.Password,
		DelayMs:   a.cfg.DelayMs,
		Tally:     a.tallyTextLocked(),
		Info:      a.info,
	}
	if a.best != nil {
		s.ChromaName, s.ChromaCapsule, s.ChromaChance = a.best.Name, a.best.Capsule, a.best.Chance
	}
	return s
}

var rarityOrder = []string{"Uncommon", "Rare", "Epic", "Legendary", "Chroma", "Mystical", "Unique"}

// tallyTextLocked lists what has been opened, rarest last, one line per item.
func (a *App) tallyTextLocked() string {
	if len(a.tally) == 0 {
		return "Nothing opened yet."
	}
	seen := map[string]bool{}
	order := make([]string, 0, len(a.tally))
	for _, r := range rarityOrder {
		if _, ok := a.tally[r]; ok {
			order = append(order, r)
			seen[r] = true
		}
	}
	var rest []string
	for r := range a.tally {
		if !seen[r] {
			rest = append(rest, r)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)

	var sb strings.Builder
	lines := 0
	for _, r := range order {
		names := make([]string, 0, len(a.tally[r]))
		total := 0
		for n, c := range a.tally[r] {
			names = append(names, n)
			total += c
		}
		sort.Strings(names)
		fmt.Fprintf(&sb, "%s (%s)\r\n", r, fmtInt(total))
		for _, n := range names {
			if lines++; lines > 300 {
				sb.WriteString("  ...\r\n")
				return sb.String()
			}
			fmt.Fprintf(&sb, "  %s x%s\r\n", n, fmtInt(a.tally[r][n]))
		}
	}
	return sb.String()
}

// SetDelay stores the wait between opens in milliseconds.
func (a *App) SetDelay(ms float64) {
	if ms < 0 {
		ms = 0
	}
	a.mu.Lock()
	if a.cfg.DelayMs != ms {
		a.cfg.DelayMs = ms
		a.saveConfigLocked()
	}
	a.mu.Unlock()
}

func (a *App) SelectCapsule(name string) {
	a.mu.Lock()
	a.cfg.Capsule = name
	a.saveConfigLocked()
	a.mu.Unlock()
}

func (a *App) SetAutoStart(on bool) {
	a.mu.Lock()
	a.cfg.AutoStart = on
	a.saveConfigLocked()
	a.mu.Unlock()
}

func (a *App) ResetStats() {
	a.mu.Lock()
	a.opened = 0
	a.tally = map[string]map[string]int{}
	a.lastNew, a.lastRare = "NONE", "NONE"
	a.mu.Unlock()
}

func (a *App) SaveStats() {
	a.mu.Lock()
	out := map[string]any{
		"opened":   a.opened,
		"lastNew":  a.lastNew,
		"lastRare": a.lastRare,
		"tally":    a.tally,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	a.mu.Unlock()
	if err == nil {
		_ = os.WriteFile(filepath.Join(a.dir, "stats.json"), b, 0o600)
	}
}

func fmtInt(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func (s Snapshot) StatusLine() string {
	state := "Idle"
	if s.Running {
		state = fmt.Sprintf("Opening \"%s\"", s.Capsule)
	}
	return fmt.Sprintf("%s | %s opened", state, fmtInt(s.Opened))
}
