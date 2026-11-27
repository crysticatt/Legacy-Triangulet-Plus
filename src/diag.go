package main

import (
	"fmt"
	"runtime"
	"strings"
)

// Diagnostics builds a plain-text report that is safe to paste into a bug report:
// no passwords, tokens or topic names.
func Diagnostics(s Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Triangulet Plus v%s (%s/%s)\n", appVersion, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "running=%v mode=%s capsule=%s workers=%d delay=%.1fms inflight=%d\n", s.Running, s.Mode, s.Capsule, s.Workers, s.DelayMs, s.InFlight)
	fmt.Fprintf(&b, "opened=%d perMin=%d coins=%d logged-in=%v\n", s.Opened, int(s.PerMin), s.Coins, s.HasAcct)
	fmt.Fprintf(&b, "collected=%d/%d\n", s.Collected, s.TotalTrians)
	fmt.Fprintf(&b, "autoUpdate=%v keepAwake=%v theme=%s downtime=%q paused=%v\n", s.AutoUpdate, s.KeepAwake, s.Theme, s.DownText, s.Down)
	fmt.Fprintf(&b, "status: %s\n", s.Info)
	fmt.Fprintf(&b, "ETA rate=%.1f/s luckPulls=%d/%d\n", s.ETA.Rate, s.Luck.Pulls, s.Luck.AllPulls)
	for _, g := range s.ETA.Groups {
		fmt.Fprintf(&b, "  %s: missing=%d noOdds=%d packs=%.0f hardest=%s (%.3f%%)\n", g.Name, g.Missing, g.NoOdds, g.Packs, g.Hardest, g.HardestP)
	}
	for _, r := range s.Luck.Rows {
		fmt.Fprintf(&b, "  luck %s: got %d, expected %.1f, luckier than %.0f%%\n", r.Rarity, r.Actual, r.Expected, r.Pct)
	}
	return b.String()
}
