package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// loginServer only knows POST <path> with {<field>: user, password: pass}.
func loginServer(path, field, user, pass, token string, opens *int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == path && r.Method == "POST":
			var b map[string]string
			json.NewDecoder(r.Body).Decode(&b)
			if _, has := b[field]; !has {
				w.WriteHeader(400)
				return
			}
			if b[field] != user || b["password"] != pass {
				w.WriteHeader(401)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]string{"token": token}})
		case r.URL.Path == "/api/open":
			if r.Header.Get("authorization") != token {
				w.WriteHeader(401)
				return
			}
			atomic.AddInt64(opens, 1)
			json.NewEncoder(w).Encode(map[string]any{"rarity": "Rare", "trian": "X", "new": false})
		default:
			w.WriteHeader(404)
		}
	}))
}

func TestLoginDiscoversEndpoint(t *testing.T) {
	var opens int64
	srv := loginServer("/api/auth/login", "username", "crysticat", "hunter2", testJWT, &opens)
	defer srv.Close()

	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	if err := a.SetCredentials("crysticat", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := a.Login(); err != nil {
		t.Fatalf("login: %v", err)
	}
	s := a.Snapshot()
	if !s.HasToken || !strings.Contains(s.Info, "Logged in") {
		t.Fatalf("snapshot = %+v", s)
	}
	if a.cfg.LoginPath != "/api/auth/login" || a.cfg.LoginField != "username" {
		t.Fatalf("remembered %q %q", a.cfg.LoginPath, a.cfg.LoginField)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	var opens int64
	srv := loginServer("/api/login", "name", "crysticat", "hunter2", testJWT, &opens)
	defer srv.Close()

	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.SetCredentials("crysticat", "wrong")
	err := a.Login()
	if err == nil || !strings.Contains(err.Error(), "wrong username or password") {
		t.Fatalf("err = %v", err)
	}
	if a.Snapshot().HasToken {
		t.Fatal("should not have a token")
	}
}

func TestLoginNoEndpoint(t *testing.T) {
	var opens int64
	srv := loginServer("/somewhere/else", "name", "u", "p", testJWT, &opens)
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.SetCredentials("u", "p")
	err := a.Login()
	if err == nil || !strings.Contains(err.Error(), "loginPath") {
		t.Fatalf("err = %v", err)
	}
}

func TestCredentialsArePlainTextInConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRIOPENER_DIR", dir)
	a := NewApp(dataDir())
	if err := a.SetCredentials("someone", ""); err == nil {
		t.Fatal("expected error when no password given")
	}
	if err := a.SetCredentials("someone", "pw123"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"password": "pw123"`) || !strings.Contains(string(b), `"username": "someone"`) {
		t.Fatalf("config.json should hold the login in plain text:\n%s", b)
	}
	// editing the file by hand is picked up on the next launch
	edited := strings.Replace(string(b), "pw123", "newpass", 1)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(edited), 0o600)
	a2 := NewApp(dataDir())
	if s := a2.Snapshot(); s.Password != "newpass" || s.Username != "someone" || !s.HasCreds {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestSetDelayAndTally(t *testing.T) {
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.SetDelay(40)
	if a.Snapshot().DelayMs != 40 {
		t.Fatal("delay not stored")
	}
	if !strings.Contains(a.Snapshot().Tally, "Nothing opened") {
		t.Fatal("empty tally text wrong")
	}
	a.record(openResp{Rarity: "Rare", Trian: "A", New: true}, "OG")
	a.record(openResp{Rarity: "Rare", Trian: "A"}, "OG")
	a.record(openResp{Rarity: "Mystical", Trian: "B"}, "OG")
	tl := a.Snapshot().Tally
	if !strings.Contains(tl, "Rare (2)") || !strings.Contains(tl, "A x2") || strings.Index(tl, "Rare") > strings.Index(tl, "Mystical") {
		t.Fatalf("tally = %q", tl)
	}
}

func TestAutoReloginWhenTokenRejected(t *testing.T) {
	var opens int64
	srv := loginServer("/api/login", "name", "u", "p", testJWT, &opens)
	defer srv.Close()

	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.cfg.DelayMs = 1
	a.SetCredentials("u", "p")
	a.mu.Lock()
	a.setTokenLocked("eyJhbGciOiJIUzI1NiJ9.eyJ4IjoxfQ.stale") // old/invalid token
	a.cfg.Capsule = "OG"
	a.mu.Unlock()

	a.StartAsync()
	time.Sleep(400 * time.Millisecond)
	a.Stop()
	time.Sleep(150 * time.Millisecond) // let the loop goroutine finish before the temp dir is removed
	if n := atomic.LoadInt64(&opens); n < 5 {
		t.Fatalf("expected opens after auto re-login, got %d (info=%q notice=%q)", n, a.Snapshot().Info, a.Snapshot().Notice)
	}
}

func TestStartAsyncLogsInWhenNoToken(t *testing.T) {
	var opens int64
	srv := loginServer("/api/login", "name", "u", "p", testJWT, &opens)
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.cfg.DelayMs = 1
	a.SetCredentials("u", "p")
	a.SelectCapsule("OG")
	a.StartAsync()
	time.Sleep(400 * time.Millisecond)
	a.Stop()
	time.Sleep(150 * time.Millisecond) // let the loop goroutine finish before the temp dir is removed
	if atomic.LoadInt64(&opens) == 0 {
		t.Fatalf("no opens; info=%q", a.Snapshot().Info)
	}
}

// A "no" from the first address/field tried must not end the search when none has worked yet.
func TestLoginKeepsLookingAfterFirstNo(t *testing.T) {
	var opens int64
	// the site only knows field "username": the first try ("name") answers 401
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" && r.Method == "POST" {
			var b map[string]string
			json.NewDecoder(r.Body).Decode(&b)
			if b["username"] != "crysticat" || b["password"] != "hunter2" {
				w.WriteHeader(401)
				w.Write([]byte(`{"error":"Invalid username"}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"token": testJWT})
			return
		}
		_ = opens
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.SetCredentials("crysticat", "hunter2")
	if err := a.Login(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if a.cfg.LoginField != "username" {
		t.Fatalf("remembered field %q", a.cfg.LoginField)
	}
}

func TestLoginShowsSiteReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"Invalid username"}`))
	}))
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.SetCredentials("u", "p")
	err := a.Login()
	if err == nil || !errors.Is(err, errRejected) || !strings.Contains(err.Error(), "Invalid username") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

// A plain 403 (a block, not a bad password) is not reported as a wrong password and is retried.
func TestLogin403BlockIsNotWrongPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`<html>Forbidden</html>`))
	}))
	defer srv.Close()
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	a := NewApp(dataDir())
	a.cfg.BaseURL = srv.URL
	a.SetCredentials("u", "p")
	err := a.Login()
	if err == nil || errors.Is(err, errRejected) || !strings.Contains(err.Error(), "refused the login") {
		t.Fatalf("err = %v", err)
	}
}
