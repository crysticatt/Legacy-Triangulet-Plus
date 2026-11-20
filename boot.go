package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// "Before sign-in" mode. Windows only starts a person's startup apps once they sign in,
// so for opening to begin the moment the PC powers on there is a second way in: a
// scheduled task (trigger "at startup", runs as SYSTEM) that starts this program with
// --headless. Headless has no window or tray icon; it just logs in with the saved
// details and opens. When the person signs in and the normal app starts, it leaves a
// "takeover" note in the data folder, headless sees it and quits, and the normal app
// carries on opening.

const bootTaskName = "TrianguletOpenerBoot"

// bootCreateArgs is the schtasks command line that creates the task.
func bootCreateArgs(exe, dataDir string) string {
	tr := `\"` + exe + `\" --headless --data \"` + strings.TrimRight(dataDir, `\/`) + `\"`
	return `/Create /TN ` + bootTaskName + ` /TR "` + tr + `" /SC ONSTART /RU SYSTEM /RL HIGHEST /F`
}

func bootDeleteArgs() string { return `/Delete /TN ` + bootTaskName + ` /F` }

func takeoverPath(dir string) string { return filepath.Join(dir, "takeover.flag") }

// requestTakeover tells a headless copy (if any) that the normal app is now running.
func requestTakeover(dir string) {
	_ = os.WriteFile(takeoverPath(dir), []byte(time.Now().Format(time.RFC3339Nano)), 0o600)
}

// takeoverRequested is true if a note newer than `since` exists (older notes are leftovers).
func takeoverRequested(dir string, since time.Time) bool {
	st, err := os.Stat(takeoverPath(dir))
	return err == nil && st.ModTime().After(since)
}

// RunHeadless opens in the background until the normal app takes over (or stop closes).
func RunHeadless(h *Hub, poll time.Duration, stop <-chan struct{}) {
	started := time.Now().Add(-time.Second)
	cfg := h.cfg()
	if (cfg.Username != "" && cfg.Password != "") || cfg.Token != "" {
		h.Start()
	}
	for {
		select {
		case <-stop:
			h.Close()
			return
		case <-time.After(poll):
		}
		if takeoverRequested(h.primary.dir, started) {
			h.Close()
			_ = os.Remove(takeoverPath(h.primary.dir))
			return
		}
	}
}
