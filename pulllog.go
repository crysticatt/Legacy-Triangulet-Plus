package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"time"
)

// PullEvent is one entry of the pull log: every Epic-or-better pull and every new trian.
type PullEvent struct {
	Time    time.Time
	Trian   string
	Rarity  string
	Capsule string
	New     bool
}

const logKeep = 300

func notable(r openResp) bool {
	switch r.Rarity {
	case "Epic", "Legendary", "Chroma", "Mystical", "Unique":
		return true
	}
	return r.New
}

// logPull records a notable pull in memory and appends it to pull-log.csv in the data folder.
func (h *Hub) logPull(capsule string, r openResp) {
	if !notable(r) {
		return
	}
	ev := PullEvent{Time: time.Now(), Trian: r.Trian, Rarity: r.Rarity, Capsule: capsule, New: r.New}
	h.mu.Lock()
	h.log = append(h.log, ev)
	if len(h.log) > logKeep {
		h.log = h.log[len(h.log)-logKeep:]
	}
	h.mu.Unlock()
	h.appendLogCSV(ev)
}

func (h *Hub) logPath() string { return filepath.Join(h.primary.dir, "pull-log.csv") }

func (h *Hub) appendLogCSV(ev PullEvent) {
	h.logMu.Lock()
	defer h.logMu.Unlock()
	_, statErr := os.Stat(h.logPath())
	f, err := os.OpenFile(h.logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if statErr != nil {
		_ = w.Write([]string{"time", "rarity", "trian", "capsule", "new"})
	}
	n := "no"
	if ev.New {
		n = "yes"
	}
	_ = w.Write([]string{ev.Time.Format("2006-01-02 15:04:05"), ev.Rarity, ev.Trian, ev.Capsule, n})
	w.Flush()
}

// recentLog returns the newest entries first.
func (h *Hub) recentLog(n int) []PullEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]PullEvent, 0, n)
	for i := len(h.log) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, h.log[i])
	}
	return out
}

// ---------- missing-trians tracker ----------

// collection works out how many of the game's trians are owned and which are missing.
func collection(inv []InvItem) (collected int, missing []InvItem, owned map[string]bool) {
	owned = map[string]bool{}
	for _, it := range inv {
		owned[lowerKey(it.Name)] = true
	}
	for _, t := range builtinTrians {
		if owned[lowerKey(t.Name)] {
			collected++
		} else {
			missing = append(missing, InvItem{Name: t.Name, Capsule: t.Capsule, Rarity: t.Rarity})
		}
	}
	sortInventory(missing, nil)
	return
}

func lowerKey(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// ---------- alerts ----------

// alertHook plays a sound for an alert ("rare" or "warn"); the Windows build sets it.
var alertHook func(kind string)

// alert queues a tray notification and (if enabled) plays a sound.
func (h *Hub) alert(kind, title, text string) {
	h.primary.pushBalloon(title, text)
	if hook := alertHook; hook != nil && h.cfg().AlertSound {
		hook(kind)
	}
}

// checkAlerts runs every supervisor tick while opening: low coins, out of coins.
func (h *Hub) checkAlerts() {
	cfg := h.cfg()
	coins := h.pool.get()
	if coins < 0 || cfg.CoinCost < 1 {
		return
	}
	packs := int(coins) / cfg.CoinCost
	h.mu.Lock()
	lowWarned, outWarned := h.lowWarned, h.outWarned
	ws := append([]*App(nil), h.workers...)
	h.mu.Unlock()

	if cfg.AlertLowPacks > 0 {
		switch {
		case packs < cfg.AlertLowPacks && packs > 0 && !lowWarned:
			h.mu.Lock()
			h.lowWarned = true
			h.mu.Unlock()
			h.alert("warn", "Coins running low", fmtInt(packs)+" packs left")
		case packs >= cfg.AlertLowPacks*2 && lowWarned:
			h.mu.Lock()
			h.lowWarned = false
			h.mu.Unlock()
		}
	}
	allOut := true
	for _, w := range ws {
		w.mu.Lock()
		if !w.outCoins {
			allOut = false
		}
		w.mu.Unlock()
	}
	switch {
	case allOut && !outWarned:
		h.mu.Lock()
		h.outWarned = true
		h.mu.Unlock()
		h.alert("warn", "Out of coins", "Opening paused. It resumes by itself when you have 25 coins again.")
	case !allOut && outWarned:
		h.mu.Lock()
		h.outWarned = false
		h.mu.Unlock()
	}
}
