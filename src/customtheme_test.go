package main

import "testing"

func TestCustomThemes(t *testing.T) {
	if c, ok := parseHex("#1A2b3C"); !ok || c != RGB(0x1a, 0x2b, 0x3c) {
		t.Fatalf("parseHex: %v %v", c, ok)
	}
	if c, ok := parseHex("f80"); !ok || c != RGB(255, 136, 0) {
		t.Fatalf("short hex: %v", c)
	}
	if _, ok := parseHex("#12345"); ok {
		t.Fatal("bad hex accepted")
	}
	// a dark base gets dark-style derived colours, a light base light-style
	d := deriveTheme(RGB(10, 10, 20), RGB(30, 30, 50), RGB(255, 80, 80), RGB(240, 240, 255))
	l := deriveTheme(RGB(240, 240, 250), RGB(255, 255, 255), RGB(0, 100, 255), RGB(20, 20, 40))
	if lumaOf(d.Field) >= lumaOf(d.Bg) || lumaOf(l.Field) <= lumaOf(l.Bg) {
		t.Fatal("field colour goes the wrong way")
	}
	if d.Good != themes["dark"].Good || l.Good != themes["light"].Good {
		t.Fatal("status colours not taken from the matching base theme")
	}

	h := NewHub(t.TempDir())
	if err := h.SaveCustomTheme(CustomTheme{Name: "  Sunset  ", Bg: "#201020", Card: "#301830", Accent: "#ff7a30", Text: "#ffeedd"}); err != nil {
		t.Fatal(err)
	}
	if got := normTheme("custom:sunset"); got != "custom:sunset" {
		t.Fatalf("custom theme not selectable: %q", got)
	}
	h.SetTheme("custom:sunset")
	s := h.Snapshot()
	if s.Theme != "custom:sunset" || s.ThemeVars["--accent"] != "#ff7a30" || len(s.CustomThemes) != 1 {
		t.Fatalf("snapshot: %q %v %d", s.Theme, s.ThemeVars["--accent"], len(s.CustomThemes))
	}
	// same name (any case) replaces
	if err := h.SaveCustomTheme(CustomTheme{Name: "SUNSET", Bg: "#000000", Card: "#111111", Accent: "#00ff00", Text: "#ffffff"}); err != nil {
		t.Fatal(err)
	}
	if n := len(h.Snapshot().CustomThemes); n != 1 {
		t.Fatalf("expected 1 theme, have %d", n)
	}
	if err := h.SaveCustomTheme(CustomTheme{Name: "", Bg: "#000000", Card: "#111111", Accent: "#00ff00", Text: "#ffffff"}); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := h.SaveCustomTheme(CustomTheme{Name: "x", Bg: "nope", Card: "#111111", Accent: "#00ff00", Text: "#ffffff"}); err == nil {
		t.Fatal("bad colour accepted")
	}
	h.DeleteCustomTheme("sunset")
	if s := h.Snapshot(); s.Theme != "dark" || len(s.CustomThemes) != 0 {
		t.Fatalf("after delete: %q %d", s.Theme, len(s.CustomThemes))
	}
	if normTheme("custom:sunset") != "dark" {
		t.Fatal("deleted theme still selectable")
	}
}
