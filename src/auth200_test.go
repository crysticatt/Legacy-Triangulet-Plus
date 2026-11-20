package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This site answers a bad token with HTTP 200 {"message":"unauthorized"}.
func quirkyServer(wantAuth string, loginToken string, opens *int64) *httptest.Server {
	unauthorized := func(w http.ResponseWriter) { w.Write([]byte(`{"message":"unauthorized"}`)) }
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			if loginToken == "" {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"token": loginToken})
		case "/data/trians":
			if r.Header.Get("authorization") != wantAuth {
				unauthorized(w)
				return
			}
			w.Write([]byte(`{"ValuesnCapsules":[{"name":"OG"}]}`))
		case "/api/open":
			if r.Header.Get("authorization") != wantAuth {
				unauthorized(w)
				return
			}
			atomic.AddInt64(opens, 1)
			w.Write([]byte(`{"rarity":"Rare","trian":"X","new":false}`))
		default:
			w.WriteHeader(404)
		}
	}))
}

func TestUnauthorized200TriesOtherTokenForm(t *testing.T) {
	var opens int64
	srv := quirkyServer("triangulet "+testJWT, "", &opens) // site wants the prefixed form
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.cfg.DelayMs = 1
	a.SetToken(testJWT) // we only have the bare token
	a.SelectCapsule("OG")
	if err := a.LoadCapsules(); err != nil {
		t.Fatalf("LoadCapsules: %v", err)
	}
	a.Start()
	time.Sleep(300 * time.Millisecond)
	a.Stop()
	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt64(&opens) < 5 {
		t.Fatalf("opens=%d notice=%q", opens, a.Snapshot().Notice)
	}
}

func TestRelogin200Unauthorized(t *testing.T) {
	var opens int64
	srv := quirkyServer(testJWT, testJWT, &opens)
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.cfg.DelayMs = 1
	a.SetCredentials("u", "p")
	a.mu.Lock()
	a.setTokenLocked("eyJhbGciOiJIUzI1NiJ9.eyJ4IjoxfQ.stale")
	a.cfg.Capsule = "OG"
	a.mu.Unlock()
	a.Start()
	time.Sleep(400 * time.Millisecond)
	a.Stop()
	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt64(&opens) < 5 {
		s := a.Snapshot()
		t.Fatalf("opens=%d notice=%q info=%q", opens, s.Notice, s.Info)
	}
}

func TestLoginReportsTokenNotAccepted(t *testing.T) {
	var opens int64
	// login hands back a token, but the site never accepts it
	srv := quirkyServer("some-other-token", testJWT, &opens)
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.cfg.LoginPath = "/api/login"
	a.SetCredentials("u", "p")
	err := a.Login()
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("err = %v", err)
	}
}

func TestIsAuthFailure(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{401, "", true},
		{403, "x", true},
		{200, `{"message":"unauthorized"}`, true},
		{200, `{"message":"Unauthorized"}`, true},
		{200, `{"rarity":"Rare","trian":"X"}`, false},
		{500, "boom", false},
	}
	for _, c := range cases {
		if got := isAuthFailure(c.status, []byte(c.body)); got != c.want {
			t.Errorf("isAuthFailure(%d,%q)=%v want %v", c.status, c.body, got, c.want)
		}
	}
}
