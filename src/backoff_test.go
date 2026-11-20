package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A few server hiccups in a row must not stop the opener.
func TestRecoversFromShortRunOfServerErrors(t *testing.T) {
	var calls, ok int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&calls, 1)
		if n <= 8 { // 8 failures, more than a naive "5 in a row" limit would allow
			w.WriteHeader(503)
			w.Write([]byte("busy, try again"))
			return
		}
		atomic.AddInt64(&ok, 1)
		w.Write([]byte(`{"rarity":"Rare","trian":"X","new":false}`))
	}))
	defer srv.Close()

	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.cfg.DelayMs = 1
	a.cfg.MaxErrorsInRow = 25
	a.errBackoff = time.Millisecond
	a.SetToken(testJWT)
	a.SelectCapsule("OG")
	a.Start()
	time.Sleep(400 * time.Millisecond)
	a.Stop()
	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt64(&ok) < 5 {
		t.Fatalf("did not recover: ok=%d notice=%q", ok, a.Snapshot().Notice)
	}
}

func TestErrorShowsServerMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":"You don't have enough tokens"}`)) // 200 but not a pack result
	}))
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.SetToken(testJWT)
	_, err := a.openOnce("OG")
	if err == nil || !strings.Contains(err.Error(), "enough tokens") {
		t.Fatalf("err = %v", err)
	}
}
