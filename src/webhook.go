package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// webhookText posts a plain message (an embed with a title and text) to the Discord webhook, if one is set.
func (h *Hub) webhookText(title, text string) {
	cfg := h.cfg()
	if cfg.WebhookURL == "" {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"username": "Triangulet Plus",
		"embeds":   []map[string]any{{"title": title, "description": text, "color": 0x3B8CFF}},
	})
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(cfg.WebhookURL, "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

// webhook posts a rare pull to a Discord webhook address, if one is set in config.json.
func (h *Hub) webhook(r openResp, capsule string) {
	cfg := h.cfg()
	if cfg.WebhookURL == "" {
		return
	}
	desc := fmt.Sprintf("**%s** (%s) from the **%s** capsule", r.Trian, r.Rarity, capsule)
	body, _ := json.Marshal(map[string]any{
		"username": "Triangulet Plus",
		"embeds": []map[string]any{{
			"title":       r.Rarity + " pulled!",
			"description": desc,
			"color":       map[string]int{"Mystical": 0xFF64BE, "Chroma": 0x50F0E6}[r.Rarity],
		}},
	})
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Post(cfg.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	resp.Body.Close()
}
