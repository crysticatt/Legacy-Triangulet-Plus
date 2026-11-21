package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Phone alerts through ntfy (https://ntfy.sh): the opener posts a short text to a "topic" and the ntfy
// app on your phone shows it. Anyone who knows the topic name can read it, so use a long random one.

const defaultNtfyServer = "https://ntfy.sh"

var topicRe = regexp.MustCompile(`^[A-Za-z0-9_-]{4,64}$`)

func newNtfyTopic() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "tri-" + hex.EncodeToString(b)
}

func ntfyBase(cfg Config) string {
	s := strings.TrimRight(strings.TrimSpace(cfg.NtfyServer), "/")
	if s == "" {
		s = defaultNtfyServer
	}
	return s
}

func sendNtfy(base, topic, title, text, tags string, priority int) error {
	if !topicRe.MatchString(topic) {
		return errors.New("the topic must be 4-64 letters, numbers, - or _")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("ntfy server address is not valid")
	}
	req, err := http.NewRequest("POST", base+"/"+topic, strings.NewReader(text))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Tags", tags)
	req.Header.Set("Priority", fmt.Sprint(priority))
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ntfy answered HTTP %d", resp.StatusCode)
	}
	return nil
}

// ntfyAlert sends an alert to the phone (no-op unless turned on and a topic is set).
func (h *Hub) ntfyAlert(kind, title, text string) {
	cfg := h.cfg()
	if !cfg.NtfyOn || cfg.NtfyTopic == "" {
		return
	}
	tags, pr := "tada", 4
	switch kind {
	case "warn":
		tags, pr = "warning", 3
	case "info":
		tags, pr = "bar_chart", 2
	}
	if err := sendNtfy(ntfyBase(cfg), cfg.NtfyTopic, title, text, tags, pr); err != nil {
		h.primary.logf("ntfy: %v", err)
	}
}

// NtfyTest sends a test message and returns what happened, for the Settings page.
func (h *Hub) NtfyTest() string {
	cfg := h.cfg()
	if cfg.NtfyTopic == "" {
		return "Type a topic name first (or press Random)."
	}
	if err := sendNtfy(ntfyBase(cfg), cfg.NtfyTopic, "Triangulet Opener", "Phone alerts work.", "white_check_mark", 3); err != nil {
		return "Could not send: " + err.Error()
	}
	return "Sent. Check the ntfy app on your phone."
}

func (h *Hub) SetNtfy(on bool, topic string) string {
	topic = strings.TrimSpace(topic)
	if topic != "" && !topicRe.MatchString(topic) {
		return "The topic must be 4-64 letters, numbers, - or _"
	}
	h.update(func(c *Config) { c.NtfyOn, c.NtfyTopic = on && topic != "", topic })
	return ""
}

func (h *Hub) SetNtfyOn(on bool) string {
	return h.SetNtfy(on, h.cfg().NtfyTopic)
}

func (h *Hub) RandomNtfyTopic() { h.update(func(c *Config) { c.NtfyTopic = newNtfyTopic() }) }

func (h *Hub) SetTheme(name string) {
	h.update(func(c *Config) { c.Theme = normTheme(name) })
}
