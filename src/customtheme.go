package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// CustomTheme is a theme the person made in Settings: four colours, everything else is worked out.
type CustomTheme struct {
	Name   string `json:"name"`
	Bg     string `json:"bg"`     // window background, "#rrggbb"
	Card   string `json:"card"`   // panels and cards
	Accent string `json:"accent"` // buttons, highlights
	Text   string `json:"text"`   // main text
}

const maxCustomThemes = 12

var themesMu sync.RWMutex

func parseHex(s string) (Col, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return Col{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return Col{}, false
	}
	return RGB(uint8(v>>16), uint8(v>>8), uint8(v)), true
}

func colHex(c Col) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func mixCol(a, b Col, t float64) Col {
	f := func(x, y uint8) uint8 {
		v := float64(x) + (float64(y)-float64(x))*t + 0.5
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		return uint8(v)
	}
	return RGB(f(a.R, b.R), f(a.G, b.G), f(a.B, b.B))
}

func lumaOf(c Col) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// deriveTheme works out the whole palette from four colours.
func deriveTheme(bg, card, accent, text Col) Theme {
	light := lumaOf(bg) > 0.5
	black, white := RGB(0, 0, 0), RGB(255, 255, 255)
	base := themes["dark"]
	if light {
		base = themes["light"]
	}
	t := Theme{
		Bg: bg, Card: card, Accent: accent, Text: text,
		Panel:   mixCol(bg, card, 0.5),
		CardHi:  mixCol(card, text, 0.07),
		Edge:    mixCol(card, text, 0.2),
		AccentD: mixCol(accent, black, 0.25),
		Muted:   mixCol(text, bg, 0.4),
		Dim:     mixCol(text, bg, 0.62),
		Good:    base.Good, Bad: base.Bad, BadD: base.BadD, Warn: base.Warn,
	}
	if light {
		t.Field = mixCol(bg, white, 0.35)
		t.Cyan = mixCol(accent, black, 0.2)
		t.Track = mixCol(card, text, 0.35)
	} else {
		t.Field = mixCol(bg, black, 0.35)
		t.Cyan = mixCol(accent, white, 0.4)
		t.Track = mixCol(card, text, 0.12)
	}
	return t
}

func (c CustomTheme) colors() (bg, card, accent, text Col, err error) {
	var ok [4]bool
	bg, ok[0] = parseHex(c.Bg)
	card, ok[1] = parseHex(c.Card)
	accent, ok[2] = parseHex(c.Accent)
	text, ok[3] = parseHex(c.Text)
	for _, k := range ok {
		if !k {
			return bg, card, accent, text, errors.New("colours must look like #1a2b3c")
		}
	}
	return
}

func customKey(name string) string { return "custom:" + strings.ToLower(strings.TrimSpace(name)) }

func cleanName(n string) string {
	n = strings.Join(strings.Fields(n), " ")
	if r := []rune(n); len(r) > 24 {
		n = string(r[:24])
	}
	return n
}

// registerCustom makes the saved custom themes selectable (and forgets removed ones).
func registerCustom(list []CustomTheme) {
	themesMu.Lock()
	defer themesMu.Unlock()
	for k := range themes {
		if strings.HasPrefix(k, "custom:") {
			delete(themes, k)
		}
	}
	for _, c := range list {
		if bg, card, ac, tx, err := c.colors(); err == nil && cleanName(c.Name) != "" {
			themes[customKey(c.Name)] = deriveTheme(bg, card, ac, tx)
		}
	}
}

// themeVars is the palette as CSS variables for the dashboard page.
func themeVars(t Theme) map[string]string {
	light := lumaOf(t.Bg) > 0.5
	knob := colHex(t.Muted)
	if light {
		knob = "#ffffff"
	}
	return map[string]string{
		"--bg": colHex(t.Bg), "--panel": colHex(t.Panel), "--card": colHex(t.Card), "--edge": colHex(t.Edge), "--field": colHex(t.Field),
		"--accent": colHex(t.Accent), "--accentd": colHex(t.AccentD), "--cyan": colHex(t.Cyan), "--text": colHex(t.Text),
		"--muted": colHex(t.Muted), "--dim": colHex(t.Dim), "--good": colHex(t.Good), "--bad": colHex(t.Bad), "--badd": colHex(t.BadD),
		"--warn": colHex(t.Warn), "--cardhi": colHex(t.CardHi), "--track": colHex(t.Track), "--goodbg": colHex(mixCol(t.Card, t.Good, 0.2)),
		"--knob": knob,
	}
}

// SaveCustomTheme adds a custom theme, or replaces the one with the same name.
func (h *Hub) SaveCustomTheme(c CustomTheme) error {
	c.Name = cleanName(c.Name)
	if c.Name == "" {
		return errors.New("give the theme a name")
	}
	if _, _, _, _, err := c.colors(); err != nil {
		return err
	}
	var full bool
	h.update(func(cfg *Config) {
		k := customKey(c.Name)
		for i, x := range cfg.CustomThemes {
			if customKey(x.Name) == k {
				cfg.CustomThemes[i] = c
				return
			}
		}
		if len(cfg.CustomThemes) >= maxCustomThemes {
			full = true
			return
		}
		cfg.CustomThemes = append(append([]CustomTheme{}, cfg.CustomThemes...), c)
	})
	if full {
		return fmt.Errorf("you can keep up to %d custom themes: delete one first", maxCustomThemes)
	}
	return nil
}

// DeleteCustomTheme removes a custom theme; if it was in use the theme goes back to dark.
func (h *Hub) DeleteCustomTheme(name string) {
	h.update(func(cfg *Config) {
		k := customKey(name)
		var keep []CustomTheme
		for _, x := range cfg.CustomThemes {
			if customKey(x.Name) != k {
				keep = append(keep, x)
			}
		}
		cfg.CustomThemes = keep
		if strings.ToLower(cfg.Theme) == k {
			cfg.Theme = "dark"
		}
	})
}
