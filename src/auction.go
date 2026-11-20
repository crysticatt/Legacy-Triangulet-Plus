package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// AddAuction lists trians on the auction with the same request the site's button sends:
// POST /api/addauction {trian, startbid, amount[, payout]}. The site may want numbers or
// strings, so numbers are tried first and strings if that is refused.
func (a *App) AddAuction(trian string, amount, startBid, payout int) error {
	if strings.TrimSpace(trian) == "" {
		return errors.New("pick a trian first")
	}
	if amount < 1 {
		return errors.New("the amount must be at least 1")
	}
	if startBid < 0 {
		return errors.New("the starting bid can't be negative")
	}
	build := func(asString bool) []byte {
		v := func(n int) any {
			if asString {
				return fmt.Sprint(n)
			}
			return n
		}
		m := map[string]any{"trian": trian, "startbid": v(startBid), "amount": v(amount)}
		if payout > 0 {
			m["payout"] = v(payout)
		}
		b, _ := json.Marshal(m)
		return b
	}
	var last error
	for _, asString := range []bool{false, true} {
		resp, err := a.do("POST", "/api/addauction", build(asString))
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if isAuthFailure(resp.StatusCode, raw) {
			return authError{resp.StatusCode}
		}
		msg := auctionReply(raw)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && !looksFailed(msg) {
			return nil
		}
		last = fmt.Errorf("%s", firstNonEmpty(msg, fmt.Sprintf("HTTP %d", resp.StatusCode)))
	}
	return last
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

func auctionReply(raw []byte) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		for _, k := range []string{"error", "message", "msg"} {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func looksFailed(msg string) bool {
	l := strings.ToLower(msg)
	for _, w := range []string{"error", "invalid", "not enough", "don't", "do not", "cannot", "can't", "fail", "must", "unauthorized"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

// StartAuction runs AddAuction in the background and leaves the result in the snapshot.
func (h *Hub) StartAuction(trian string, amount, startBid, payout int) {
	h.mu.Lock()
	if h.aucBusy {
		h.mu.Unlock()
		return
	}
	h.aucBusy = true
	h.aucMsg = "Listing..."
	h.mu.Unlock()
	go func() {
		err := h.primary.AddAuction(trian, amount, startBid, payout)
		msg := fmt.Sprintf("Listed %dx %s", amount, trian)
		if err != nil {
			msg = "Failed: " + err.Error()
		} else {
			_ = h.primary.SyncInventory()
		}
		h.mu.Lock()
		h.aucMsg, h.aucBusy = msg, false
		h.mu.Unlock()
	}()
}

// Login saves the credentials, logs in (retrying if no token comes back), loads capsules and inventory.
func (h *Hub) Login(user, pass string) { h.loginThen(user, pass, nil) }

func (h *Hub) loginThen(user, pass string, after func()) {
	p := h.primary
	if err := p.SetCredentials(user, pass); err != nil {
		p.mu.Lock()
		p.info = err.Error()
		p.mu.Unlock()
		return
	}
	user = strings.TrimSpace(user)
	h.update(func(c *Config) { c.Accounts = upsertAccount(c.Accounts, user, pass) })
	p.mu.Lock()
	p.info = "Logging in..."
	p.wrongLogin = false
	p.mu.Unlock()
	h.mu.Lock()
	ws := append([]*App(nil), h.workers...)
	h.mu.Unlock()
	for _, w := range ws[1:] { // other workers log in again with the new details
		w.mu.Lock()
		w.cfg.Token = ""
		w.wrongLogin = false
		w.mu.Unlock()
	}
	h.bg(func() {
		if err := p.LoginRetry(func() bool {
			select {
			case <-h.quit:
				return true
			default:
				return false
			}
		}); err == nil {
			_ = p.LoadCapsules()
			_ = p.SyncInventory()
			h.sync()
			if after != nil {
				after()
			}
		}
	})
}

// Refresh reloads the capsule list and the inventory.
func (h *Hub) Refresh() {
	p := h.primary
	go func() {
		if err := p.EnsureToken(); err != nil {
			return
		}
		if p.hasToken() {
			_ = p.LoadCapsules()
			_ = p.SyncInventory()
			h.sync()
		}
	}()
}
