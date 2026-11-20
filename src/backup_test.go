package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupRoundTrip(t *testing.T) {
	f := newFakeSite(100)
	h := newTestHub(t, f)
	defer h.Close()
	h.update(func(c *Config) {
		c.Accounts = []Account{{"alice", "pw-one"}, {"bob", "pw-two"}}
		c.Theme, c.NtfyTopic = "light", "tri-abcd1234"
	})
	h.histMu.Lock()
	h.hist = []HistBucket{{T: 3600, Pulls: 5, Coins: -1}}
	h.histMu.Unlock()
	file, err := h.ExportBackup("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(file), "pw-one") || strings.Contains(string(file), "alice") {
		t.Fatal("backup is not encrypted")
	}
	if _, err := openBackup(file, "wrong pass"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if _, err := h.ExportBackup("abc"); err == nil {
		t.Fatal("short passphrase accepted")
	}
	// a second hub (another PC) restores it
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	h2 := NewHub(dataDir())
	defer h2.Close()
	h2.primary.cfg.BaseURL = f.srv.URL
	if err := h2.ImportBackup(file, "correct horse"); err != nil {
		t.Fatal(err)
	}
	c := h2.cfg()
	if len(c.Accounts) != 2 || c.Accounts[1].Password != "pw-two" || c.Theme != "light" || c.NtfyTopic != "tri-abcd1234" {
		t.Fatalf("settings not restored: %+v", c)
	}
	if b, _ := os.ReadFile(filepath.Join(h2.primary.dir, "history.json")); !strings.Contains(string(b), "3600") {
		t.Fatal("history not restored")
	}
	if err := h2.ImportBackup([]byte("TRIBAK1\nnonsense nonsense nonsense nonsense"), "correct horse"); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestPBKDF2Vector(t *testing.T) { // RFC 7914 test vector (PBKDF2-HMAC-SHA256, c=1)
	got := pbkdf2([]byte("passwd"), []byte("salt"), 1, 16)
	want := "55ac046e56e3089fec1691c22544b605"
	if h := hexOf(got); h != want {
		t.Fatalf("pbkdf2 = %s", h)
	}
}

func hexOf(b []byte) string {
	const d = "0123456789abcdef"
	s := make([]byte, 0, len(b)*2)
	for _, x := range b {
		s = append(s, d[x>>4], d[x&15])
	}
	return string(s)
}

func TestNtfy(t *testing.T) {
	var got, title string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got, title = r.URL.Path, r.Header.Get("Title") }))
	defer srv.Close()
	if err := sendNtfy(srv.URL, "tri-12345678", "Hello", "body", "tada", 4); err != nil {
		t.Fatal(err)
	}
	if got != "/tri-12345678" || title != "Hello" {
		t.Fatalf("got %q %q", got, title)
	}
	if sendNtfy(srv.URL, "../evil", "x", "y", "", 3) == nil {
		t.Fatal("bad topic accepted")
	}
}
