package main

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBootTaskArgs(t *testing.T) {
	a := bootCreateArgs(`C:\Users\A B\Desktop\TrianguletOpener.exe`, `C:\Users\A B\AppData\Roaming\TrianguletOpener\`)
	for _, want := range []string{
		`/TN TrianguletOpenerBoot`, `/SC ONSTART`, `/RU SYSTEM`, `/F`,
		`/TR "\"C:\Users\A B\Desktop\TrianguletOpener.exe\" --headless --data \"C:\Users\A B\AppData\Roaming\TrianguletOpener\""`,
	} {
		if !strings.Contains(a, want) {
			t.Errorf("args missing %q:\n%s", want, a)
		}
	}
}

func TestHeadlessOpensThenYieldsToTheNormalApp(t *testing.T) {
	f := newFakeSite(1_000_000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) { c.Capsule = "OG"; c.Mode = "fixed"; c.Workers = 2 })
	stale := takeoverPath(h.primary.dir)
	requestTakeover(h.primary.dir) // a leftover note from before must be ignored
	time.Sleep(1100 * time.Millisecond)
	_ = stale
	done := make(chan struct{})
	go func() { RunHeadless(h, 20*time.Millisecond, make(chan struct{})); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt64(&f.opens) < 20 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt64(&f.opens) < 20 {
		t.Fatal("headless did not start opening on its own")
	}
	select {
	case <-done:
		t.Fatal("headless quit because of an old takeover note")
	default:
	}
	requestTakeover(h.primary.dir)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("headless did not yield to the normal app")
	}
}
