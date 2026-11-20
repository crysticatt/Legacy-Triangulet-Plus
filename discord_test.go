package main

import (
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

// fakeDiscord answers the handshake and records every SET_ACTIVITY it sees.
func fakeDiscord(t *testing.T, conn net.Conn, got chan map[string]any) {
	go func() {
		defer conn.Close()
		op, b, err := readFrame(conn)
		if err != nil || op != 0 {
			return
		}
		var hs map[string]any
		json.Unmarshal(b, &hs)
		got <- hs
		writeFrame(conn, 1, map[string]any{"cmd": "DISPATCH", "evt": "READY"})
		for {
			_, b, err := readFrame(conn)
			if err != nil {
				return
			}
			var m map[string]any
			json.Unmarshal(b, &m)
			got <- m
			writeFrame(conn, 1, map[string]any{"cmd": "SET_ACTIVITY", "evt": nil})
		}
	}()
}

func TestDiscordPresenceShowsAndClears(t *testing.T) {
	f := newFakeSite(100000)
	defer f.srv.Close()
	h := newTestHub(t, f)
	h.update(func(c *Config) { c.Capsule = "OG"; c.Mode = "fixed" })
	got := make(chan map[string]any, 50)
	dial := func(id string) (io.ReadWriteCloser, error) {
		a, b := net.Pipe()
		fakeDiscord(t, b, got)
		return a, nil
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { RunDiscord(h, dial, 20*time.Millisecond, stop); close(done) }()

	h.Start()
	var hs, act map[string]any
	select {
	case hs = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("no handshake")
	}
	if hs["client_id"] != defaultDiscordID {
		t.Fatalf("client id %v", hs["client_id"])
	}
	select {
	case act = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("no activity")
	}
	a := act["args"].(map[string]any)["activity"].(map[string]any)
	if a["details"] != "Opening Capsules" || a["assets"].(map[string]any)["large_image"] != "logo" {
		t.Fatalf("activity = %v", a)
	}
	h.Stop()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case m := <-got:
			if _, has := m["args"].(map[string]any)["activity"]; !has {
				close(stop)
				<-done
				return // cleared
			}
		case <-deadline:
			t.Fatal("presence was not cleared after stopping")
		}
	}
}
