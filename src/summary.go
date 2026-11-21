package main

import (
	"fmt"
	"time"
)

// An hourly summary: once the clock passes the top of an hour, say what the hour before it looked like.
// It goes to the tray / macOS notification, to ntfy if that is on, and to the Discord webhook if one is set.
// Nothing is sent for an hour in which nothing was opened.

// summaryText builds the message for one hour bucket. prev is the bucket before it (for the coin change).
func summaryText(b HistBucket, prev *HistBucket) (title, text string) {
	start := time.Unix(b.T, 0)
	title = fmt.Sprintf("Hourly summary, %s-%s", start.Format("15:04"), start.Add(time.Hour).Format("15:04"))
	text = fmt.Sprintf("%s packs opened", fmtInt(b.Pulls))
	if b.Pulls > 0 {
		text += fmt.Sprintf(" (%.1f per second)", float64(b.Pulls)/3600)
	}
	text += fmt.Sprintf(" · %d Mystical · %d Chroma", b.Mystical, b.Chroma)
	if b.Coins >= 0 {
		text += " · coins " + fmtInt(b.Coins)
		if prev != nil && prev.Coins >= 0 {
			d := b.Coins - prev.Coins
			sign := "+"
			if d < 0 {
				sign, d = "-", -d
			}
			text += " (" + sign + fmtInt(d) + ")"
		}
	}
	return
}

// summaryFor finds the bucket that starts at hour t, and the one just before it.
func (h *Hub) summaryFor(t int64) (b HistBucket, prev *HistBucket, ok bool) {
	h.histMu.Lock()
	defer h.histMu.Unlock()
	for i := range h.hist {
		if h.hist[i].T == t {
			b, ok = h.hist[i], true
			if i > 0 && h.hist[i-1].T == t-3600 {
				p := h.hist[i-1]
				prev = &p
			}
		}
	}
	return
}

// maybeSummary runs every supervisor tick and sends the summary when an hour has just ended.
func (h *Hub) maybeSummary() {
	now := hourStart(time.Now())
	h.mu.Lock()
	due := h.sumHour
	if due == 0 { // first tick after start: the hour that is running now is the first one to report
		h.sumHour = now
		h.mu.Unlock()
		return
	}
	if now <= due {
		h.mu.Unlock()
		return
	}
	h.sumHour = now
	h.mu.Unlock()
	if !h.cfg().HourlySummary {
		return
	}
	if b, prev, ok := h.summaryFor(due); ok && b.Pulls > 0 {
		title, text := summaryText(b, prev)
		h.notify("info", title, text)
	}
}

// notify is a quiet notification (no sound): tray / macOS banner, ntfy and the Discord webhook.
func (h *Hub) notify(kind, title, text string) {
	h.primary.pushBalloon(title, text)
	h.bg(func() { h.ntfyAlert(kind, title, text) })
	h.bg(func() { h.webhookText(title, text) })
}
