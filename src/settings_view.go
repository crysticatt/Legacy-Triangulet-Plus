package main

import "strings"

// The Settings page fills the content area while UIState.Settings is on.
func (p *painter) viewSettings(r Rect) {
	s, st := p.s, p.st
	x, w := r.X+14, r.W-28
	row := func(y int, title string) { p.label(x, r.Y+y, title) }
	soft := func(y int, txt string, c Col) { p.text(Rect{x, r.Y + y, w, 16}, txt, 11, false, c, 0) }

	p.text(Rect{x, r.Y + 8, 200, 22}, "Settings", 16, true, colText, 0)
	p.text(Rect{r.X + r.W - 14 - 200, r.Y + 12, 200, 16}, "version "+appVersion, 11, false, colDim, 2)

	// theme
	row(40, "THEME")
	p.seg("theme", Rect{x + 70, r.Y + 34, 270, 28}, []string{"Dark", "Light", "Blue"}, themeIndex(s.Theme))

	p.toggle("awake", Rect{x + 352, r.Y + 34, w - 352, 28}, "Keep PC awake", s.KeepAwake)

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
	row(280, "BACKUP")
	soft(298, "Saves your settings, logins and History in one encrypted file (Desktop). Import reads the newest one.", colMuted)
	bf := Rect{x, r.Y + 318, 200, 28}
	p.field(bf, "bpass")
	p.f.Edits["bpass"] = Rect{bf.X + 9, bf.Y + 6, bf.W - 18, 16}
	p.button("bexport", Rect{bf.X + 208, r.Y + 318, 64, 28}, "Export", colAccent, 0, 12)
	p.button("bimport", Rect{bf.X + 280, r.Y + 318, 64, 28}, "Import", colCard, 1, 12)
	p.text(Rect{bf.X + 352, r.Y + 318, w - 352, 28}, "passphrase (6+ characters)", 10, false, colDim, 0)

	if m := strings.TrimSpace(st.SetMsg); m != "" {
		p.text(Rect{x, r.Y + 358, w, 32}, m, 11, false, colText, 0)
	}
}
