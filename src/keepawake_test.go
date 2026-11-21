package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeepAwakeFollowsOpening(t *testing.T) {
	f := newFakeSite(1000)
	h := newTestHub(t, f)
	defer h.Close()
	var on int32
	old := setAwake
	setAwake = func(v bool) {
		if v {
			atomic.StoreInt32(&on, 1)
		} else {
			atomic.StoreInt32(&on, 0)
		}
	}
	defer func() { setAwake = old }()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { KeepAwakeLoop(h, 15*time.Millisecond, stop); close(done) }()
	time.Sleep(60 * time.Millisecond)
	if atomic.LoadInt32(&on) != 0 {
		t.Fatal("awake while not opening")
	}
	h.primary.cfg.Capsule = "OG"
	h.Start()
	waitFor(t, 3*time.Second, "awake while opening", func() bool { return atomic.LoadInt32(&on) == 1 })
	h.update(func(c *Config) { c.KeepAwake = false })
	waitFor(t, 3*time.Second, "setting off releases", func() bool { return atomic.LoadInt32(&on) == 0 })
	h.update(func(c *Config) { c.KeepAwake = true })
	waitFor(t, 3*time.Second, "setting on again", func() bool { return atomic.LoadInt32(&on) == 1 })
	h.Stop()
	waitFor(t, 3*time.Second, "released after stop", func() bool { return atomic.LoadInt32(&on) == 0 })
	close(stop)
	<-done
}

func TestOldConfigGetsAutoUpdateOnce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRIOPENER_DIR", dir)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"autoUpdate": false, "theme": "light"}`), 0o600)
	a := NewApp(dir)
	if !a.cfg.AutoUpdate || a.cfg.CfgVer != 38 || a.cfg.Theme != "light" || !a.cfg.KeepAwake {
		t.Fatalf("migration: %+v", a.cfg)
	}
	a.cfg.AutoUpdate = false // the person turns it off again: must stay off
	a.saveConfigLocked()
	if b := NewApp(dir); b.cfg.AutoUpdate {
		t.Fatal("auto update forced on a second time")
	}
}
