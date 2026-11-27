package main

import "strings"

// The Settings page fills the content area while UIState.Settings is on.
func (p *painter) viewSettings(r Rect) {
	s, st := p.s, p.st
	if st.ThemeEdit {
		p.viewThemeEditor(r)
		return
	}
	x, w := r.X+14, r.W-28
	row := func(y int, title string) { p.label(x, r.Y+y, title) }
	soft := func(y int, txt string, c Col) { p.text(Rect{x, r.Y + y, w, 16}, txt, 11, false, c, 0) }

	p.text(Rect{x, r.Y + 8, 200, 22}, "Settings", 16, true, colText, 0)
	p.text(Rect{r.X + r.W - 14 - 200, r.Y + 12, 200, 16}, "version "+appVersion, 11, false, colDim, 2)

	// theme
	row(40, "THEME")
	sel := themeIndex(s.Theme)
	if strings.HasPrefix(normTheme(s.Theme), "custom:") {
		sel = -1
	}
	p.seg("theme", Rect{x + 70, r.Y + 34, 210, 28}, []string{"Dark", "Light", "Blue"}, sel)
	p.button("themeedit", Rect{x + 288, r.Y + 34, 110, 28}, "Custom themes", colCard, 1, 11)

	p.toggle("awake", Rect{x + 406, r.Y + 34, w - 406, 28}, "Stay awake", s.KeepAwake)

	// updates
	row(76, "UPDATES")
	note := s.UpdateNote
	switch {
	case s.UpdateVer != "":
		note = "Version " + s.UpdateVer + " is available (use the green Update button at the top)"
	case note == "":
		note = "You are on version " + appVersion
	}
	soft(96, note, colMuted)
	p.button("chkupd", Rect{r.X + r.W - 14 - 120, r.Y + 72, 120, 22}, "Check for updates", colCard, 1, 11)
	hw := (w - 8) / 2
	p.toggle("updcheck", Rect{x, r.Y + 116, hw, 26}, "Look for updates", !s.NoUpdateCheck)
	p.toggle("autoupd", Rect{x + hw + 8, r.Y + 116, hw, 26}, "Install by itself (when hidden)", s.AutoUpdate)

	// security
	row(154, "PASSWORD")
	p.toggle("hourly", Rect{x + w - 190, r.Y + 148, 190, 20}, "Hourly summary", s.HourlySummary)
	if s.Protected {
		soft(172, "Saved passwords are encrypted for this PC, so a copied config.json is useless elsewhere.", colGood)
	} else {
		soft(172, "Saved passwords are kept as plain text in config.json on this system. Keep that file private.", colWarn)
	}

	// phone alerts
	row(200, "PHONE ALERTS (ntfy)")
	soft(218, "Install the ntfy app, subscribe to your topic, and rare pulls and warnings arrive on your phone.", colMuted)
	tf := Rect{x, r.Y + 238, 200, 28}
	p.field(tf, "ntfy")
	p.f.Edits["ntfy"] = Rect{tf.X + 9, tf.Y + 6, tf.W - 18, 16}
	p.button("ntfyrand", Rect{tf.X + 208, r.Y + 238, 64, 28}, "Random", colCard, 1, 11)
	p.button("ntfytest", Rect{tf.X + 280, r.Y + 238, 52, 28}, "Test", colCard, 1, 11)
	p.toggle("ntfyon", Rect{tf.X + 340, r.Y + 238, w - 340, 28}, "Send to phone", s.NtfyOn)

	// backup
	row(268, "BACKUP")
	soft(286, "Saves your settings, logins and History in one encrypted file (Desktop). Import reads the newest one.", colMuted)
	bf := Rect{x, r.Y + 306, 200, 28}
	p.field(bf, "bpass")
	p.f.Edits["bpass"] = Rect{bf.X + 9, bf.Y + 6, bf.W - 18, 16}
	p.button("bexport", Rect{bf.X + 208, r.Y + 306, 64, 28}, "Export", colAccent, 0, 12)
	p.button("bimport", Rect{bf.X + 280, r.Y + 306, 64, 28}, "Import", colCard, 1, 12)
	p.text(Rect{bf.X + 352, r.Y + 306, w - 352, 28}, "passphrase (6+ characters)", 10, false, colDim, 0)

	// downtime
	row(338, "DOWNTIME (paused hours, every day)")
	df := Rect{x, r.Y + 356, 300, 26}
	p.field(df, "down")
	p.f.Edits["down"] = Rect{df.X + 9, df.Y + 5, df.W - 18, 16}
	dn := "e.g. 3:20pm-4pm, 2am-2:30am"
	if s.Down {
		dn = "Paused now, resumes at " + s.DownEnd
	}
	p.text(Rect{df.X + 308, r.Y + 356, w - 308, 26}, dn, 10, false, colDim, 0)

	if m := strings.TrimSpace(st.SetMsg); m != "" {
		p.text(Rect{x, r.Y + 382, w, 16}, m, 11, false, colText, 0)
	}
}

// viewThemeEditor is the "make your own theme" page inside Settings.
func (p *painter) viewThemeEditor(r Rect) {
	s, st := p.s, p.st
	x, w := r.X+14, r.W-28
	p.text(Rect{x, r.Y + 8, 300, 22}, "Custom themes", 16, true, colText, 0)
	p.button("themeback", Rect{r.X + r.W - 14 - 80, r.Y + 6, 80, 26}, "Back", colCard, 1, 12)

	p.label(x, r.Y+40, "SAVED")
	if len(s.CustomThemes) == 0 {
		p.text(Rect{x, r.Y + 58, w, 18}, "None yet. Make one below and press Save.", 11, false, colDim, 0)
	}
	active := normTheme(s.Theme)
	for i, c := range s.CustomThemes {
		cw := 104
		cx, cy := x+(i%5)*(cw+6), r.Y+58+(i/5)*30
		p.chip("tsel:"+c.Name, Rect{cx, cy, cw, 26}, shorten(c.Name, 12), active == customKey(c.Name))
	}

	p.label(x, r.Y+130, "MAKE ONE")
	rows := []struct{ key, label string }{{"tname", "Name"}, {"tbg", "Background"}, {"tcard", "Cards and panels"}, {"tacc", "Accent (buttons)"}, {"ttext", "Text"}}
	for i, rw := range rows {
		y := r.Y + 152 + i*34
		p.text(Rect{x, y, 128, 28}, rw.label, 12, false, colMuted, 0)
		fw := 110
		if i == 0 {
			fw = 200
		}
		fr := Rect{x + 130, y, fw, 28}
		p.field(fr, rw.key)
		p.f.Edits[rw.key] = Rect{fr.X + 9, fr.Y + 6, fr.W - 18, 16}
		if i > 0 {
			sw := Rect{fr.X + fr.W + 8, y, 28, 28}
			c, ok := parseHex(st.ThemeHex[rw.key])
			if !ok {
				c = colDim
			}
			p.c.RoundRect(sw, 7, c)
			p.c.RoundRectStroke(sw, 7, 1, colEdge)
		}
	}
	p.text(Rect{x + 330, r.Y + 152, 190, 16}, "Start from", 11, true, colAccent, 0)
	for i, n := range []string{"Dark", "Light", "Blue"} {
		p.button("tpre:"+itoaInt(i), Rect{x + 330, r.Y + 172 + i*32, 120, 26}, n, colCard, 1, 12)
	}
	p.text(Rect{x + 330, r.Y + 272, 196, 14}, "Colours look like #1a2b3c", 10, false, colDim, 0)
	p.text(Rect{x + 330, r.Y + 286, 196, 14}, "The window updates as you type", 10, false, colDim, 0)

	by := r.Y + 334
	p.button("tsave", Rect{x, by, 110, 30}, "Save", colAccent, 0, 12)
	p.button("tdel", Rect{x + 118, by, 110, 30}, "Delete", colCard, 1, 12)
	if m := strings.TrimSpace(st.SetMsg); m != "" {
		p.text(Rect{x + 240, by, w - 240, 30}, m, 11, false, colText, 0)
	}
}
