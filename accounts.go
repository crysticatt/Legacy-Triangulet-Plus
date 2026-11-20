package main

import (
	"strings"
)

// Account is one saved login. The active one is also kept in Config.Username / Password.
type Account struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// upsertAccount adds or updates a login in the saved list.
func upsertAccount(list []Account, name, pass string) []Account {
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			list[i].Name, list[i].Password = name, pass
			return list
		}
	}
	return append(list, Account{name, pass})
}

// accountIndex finds the current login in the list (-1 if it isn't saved).
func accountIndex(list []Account, name string) int {
	for i, a := range list {
		if strings.EqualFold(a.Name, name) {
			return i
		}
	}
	return -1
}

// resetForNewAccount forgets everything that belonged to the previous login.
func (p *CoinPool) reset() {
	p.mu.Lock()
	p.v = -1
	p.mu.Unlock()
}

// Accounts returns the saved logins and which one is active.
func (h *Hub) Accounts() ([]Account, int) {
	c := h.cfg()
	list := append([]Account(nil), c.Accounts...)
	return list, accountIndex(list, c.Username)
}

// SwitchAccount moves to the saved login `delta` places along (wrapping round). If opening was on,
// it starts again once the new login has a token.
func (h *Hub) SwitchAccount(delta int) {
	list, idx := h.Accounts()
	if len(list) < 2 {
		return
	}
	n := len(list)
	next := ((idx+delta)%n + n) % n
	if idx < 0 {
		next = 0
	}
	h.switchTo(list[next])
}

func (h *Hub) switchTo(a Account) {
	was := h.wanted()
	h.Stop()
	p := h.primary
	p.mu.Lock()
	p.cfg.Token = ""
	p.hasAcct, p.acctCoins, p.acctOpened = false, 0, 0
	p.inv = nil
	p.wrongLogin = false
	p.mu.Unlock()
	h.pool.reset()
	h.mu.Lock()
	h.mystRun = 0
	h.mu.Unlock()
	h.loginThen(a.Name, a.Password, func() {
		if was {
			h.Start()
		}
	})
}

// RemoveAccount deletes the current saved login and moves to a neighbour (or just clears the fields).
func (h *Hub) RemoveAccount() {
	list, idx := h.Accounts()
	if idx < 0 {
		return
	}
	list = append(list[:idx], list[idx+1:]...)
	h.update(func(c *Config) { c.Accounts = list })
	if len(list) == 0 {
		h.Stop()
		h.primary.ForgetCredentials()
		h.primary.mu.Lock()
		h.primary.cfg.Token = ""
		h.primary.mu.Unlock()
		return
	}
	h.switchTo(list[idx%len(list)])
}
