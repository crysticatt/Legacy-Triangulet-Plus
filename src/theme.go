package main

import "strings"

// Theme is a complete set of window colours. Three are built in: dark (the original), light and blue.
type Theme struct {
	Bg, Panel, Card, CardHi, Edge, Field Col
	Accent, AccentD, Cyan                Col
	Text, Muted, Dim                     Col
	Good, Bad, BadD, Warn                Col
	Track                                Col // off switch
}

var themeNames = []string{"dark", "light", "blue"}

var themes = map[string]Theme{
	"dark": {
		Bg: RGB(10, 17, 34), Panel: RGB(13, 22, 44), Card: RGB(18, 30, 58), CardHi: RGB(24, 39, 74), Edge: RGB(34, 52, 94), Field: RGB(9, 15, 32),
		Accent: RGB(59, 140, 255), AccentD: RGB(40, 104, 214), Cyan: RGB(92, 225, 255),
		Text: RGB(236, 243, 255), Muted: RGB(140, 160, 204), Dim: RGB(92, 112, 158),
		Good: RGB(66, 214, 132), Bad: RGB(236, 84, 98), BadD: RGB(190, 56, 72), Warn: RGB(255, 196, 92),
		Track: RGB(40, 58, 100),
	},
	"light": {
		Bg: RGB(232, 237, 247), Panel: RGB(243, 246, 252), Card: RGB(255, 255, 255), CardHi: RGB(231, 238, 252), Edge: RGB(203, 213, 234), Field: RGB(247, 249, 253),
		Accent: RGB(37, 108, 224), AccentD: RGB(28, 84, 186), Cyan: RGB(8, 124, 182),
		Text: RGB(22, 32, 58), Muted: RGB(78, 94, 130), Dim: RGB(120, 134, 164),
		Good: RGB(18, 150, 84), Bad: RGB(206, 46, 66), BadD: RGB(186, 38, 58), Warn: RGB(190, 118, 8),
		Track: RGB(178, 190, 214),
	},
	"blue": {
		Bg: RGB(5, 24, 64), Panel: RGB(8, 33, 82), Card: RGB(13, 46, 104), CardHi: RGB(20, 60, 128), Edge: RGB(40, 88, 164), Field: RGB(5, 19, 50),
		Accent: RGB(72, 172, 255), AccentD: RGB(40, 130, 226), Cyan: RGB(126, 236, 255),
		Text: RGB(240, 247, 255), Muted: RGB(152, 188, 235), Dim: RGB(104, 144, 200),
		Good: RGB(72, 220, 140), Bad: RGB(244, 92, 106), BadD: RGB(196, 58, 76), Warn: RGB(255, 200, 96),
		Track: RGB(36, 78, 146),
	},
}

var colTrack = themes["dark"].Track

// curTheme is the name of the theme that is applied right now.
var curTheme = "dark"

func normTheme(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if _, ok := themes[name]; ok {
		return name
	}
	return "dark"
}

func themeIndex(name string) int {
	name = normTheme(name)
	for i, n := range themeNames {
		if n == name {
			return i
		}
	}
	return 0
}

// applyTheme switches every colour the painter uses.
func applyTheme(name string) {
	curTheme = normTheme(name)
	t := themes[curTheme]
	colBg, colPanel, colCard, colCardHi, colEdge, colField = t.Bg, t.Panel, t.Card, t.CardHi, t.Edge, t.Field
	colAccent, colAccentD, colCyan = t.Accent, t.AccentD, t.Cyan
	colText, colMuted, colDim = t.Text, t.Muted, t.Dim
	colGood, colBad, colBadD, colWarn = t.Good, t.Bad, t.BadD, t.Warn
	colTrack = t.Track
}

// colorRef converts a colour to a Windows COLORREF (0x00BBGGRR).
func colorRef(c Col) uintptr { return uintptr(c.R) | uintptr(c.G)<<8 | uintptr(c.B)<<16 }
