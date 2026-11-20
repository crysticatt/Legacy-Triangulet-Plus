package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// InvItem is one trian the account owns.
type InvItem struct {
	Name    string
	Capsule string
	Rarity  string
	Qty     int
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		return n, err == nil
	}
	return 0, false
}

// findKey returns the first value stored under key anywhere in the JSON tree.
func findKey(v any, key string) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		if x, ok := t[key]; ok {
			return x, true
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if x, ok := findKey(t[k], key); ok {
				return x, true
			}
		}
	case []any:
		for _, e := range t {
			if x, ok := findKey(e, key); ok {
				return x, true
			}
		}
	}
	return nil, false
}

// parseAccount reads coins, capsules opened and the trian list from GET /data/user.
func parseAccount(raw []byte, chances map[string]float64) (coins, opened int, hasAcct bool, inv []InvItem) {
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return
	}
	if v, ok := findKey(root, "tokens"); ok {
		if n, ok := asInt(v); ok {
			coins, hasAcct = n, true
		}
	}
	if v, ok := findKey(root, "opened"); ok {
		if n, ok := asInt(v); ok {
			opened = n
		}
	}
	if v, ok := findKey(root, "trians"); ok {
		if list, ok := v.([]any); ok {
			for _, e := range list {
				m, ok := e.(map[string]any)
				if !ok {
					continue
				}
				name, _ := m["trian"].(string)
				if name == "" {
					name, _ = m["name"].(string)
				}
				q := 1
				for _, k := range []string{"quantity", "amount", "count"} {
					if n, ok := asInt(m[k]); ok {
						q = n
						break
					}
				}
				if name == "" || q <= 0 {
					continue
				}
				cap, rar := capsuleOf(name)
				inv = append(inv, InvItem{Name: name, Capsule: cap, Rarity: rar, Qty: q})
			}
		}
	}
	sortInventory(inv, chances)
	return
}

var rarityRank = map[string]int{"Mystical": 0, "Chroma": 1, "Legendary": 2, "Epic": 3, "Rare": 4, "Uncommon": 5, "Common": 6, "": 9}

func sortInventory(inv []InvItem, _ map[string]float64) {
	sort.SliceStable(inv, func(i, j int) bool {
		a, b := inv[i], inv[j]
		ra, rb := rarityRank[a.Rarity], rarityRank[b.Rarity]
		if ra != rb {
			return ra < rb
		}
		if a.Capsule != b.Capsule {
			return a.Capsule < b.Capsule
		}
		return a.Name < b.Name
	})
}

// SyncInventory reloads coins, capsule count and the real trian list from the site.
func (a *App) SyncInventory() error {
	resp, err := a.do("GET", "/data/user", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if isAuthFailure(resp.StatusCode, raw) {
		return authError{resp.StatusCode}
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d%s", resp.StatusCode, snippet(raw))
	}
	a.mu.Lock()
	ch := a.chances
	a.mu.Unlock()
	coins, opened, has, inv := parseAccount(raw, ch)
	a.mu.Lock()
	a.inv = inv
	if has {
		a.acctCoins, a.acctOpened, a.hasAcct = coins, opened, true
	}
	pool := a.pool
	a.mu.Unlock()
	if has && pool != nil {
		pool.set(int64(coins))
	}
	return nil
}
