package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a fake protector (reverses the text) so the config path can be tested on any system
func withFakeSeal(t *testing.T) {
	oa, ob := sealImpl, unsealImpl
	rev := func(s string) string {
		r := []rune(s)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return string(r)
	}
	sealImpl = func(label, p string) (string, bool) { return "fake:" + label + ":" + rev(p), true }
	unsealImpl = func(label, s string) (string, bool) {
		pre := "fake:" + label + ":"
		if !strings.HasPrefix(s, pre) {
			return "", false
		}
		return rev(strings.TrimPrefix(s, pre)), true
	}
	t.Cleanup(func() { sealImpl, unsealImpl = oa, ob })
}

func TestPasswordsNotPlainOnDisk(t *testing.T) {
	withFakeSeal(t)
	dir := t.TempDir()
	t.Setenv("TRIOPENER_DIR", dir)
	a := NewApp(dir)
	a.cfg.Username, a.cfg.Password = "u", "hunter2-secret"
	a.cfg.Accounts = []Account{{"alt", "alt-pass-123"}}
	a.cfg.Token = testJWT
	a.saveConfigLocked()
	raw, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	for _, bad := range []string{"hunter2-secret", "alt-pass-123", testJWT} {
		if strings.Contains(string(raw), bad) {
			t.Fatalf("config.json still has %q", bad)
		}
	}
	b := NewApp(dir)
	if b.cfg.Password != "hunter2-secret" || b.cfg.Accounts[0].Password != "alt-pass-123" || b.cfg.Token != testJWT {
		t.Fatalf("not restored: %+v", b.cfg)
	}
}

func TestPlainPasswordGetsProtected(t *testing.T) {
	withFakeSeal(t)
	dir := t.TempDir()
	t.Setenv("TRIOPENER_DIR", dir)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"username":"u","password":"old-plain-pw"}`), 0o600)
	a := NewApp(dir)
	if a.cfg.Password != "old-plain-pw" {
		t.Fatal("plain password not read")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	if strings.Contains(string(raw), "old-plain-pw") {
		t.Fatal("plain password was left in the file")
	}
}

func TestUnreadableSecretIsDropped(t *testing.T) {
	withFakeSeal(t)
	dir := t.TempDir()
	t.Setenv("TRIOPENER_DIR", dir)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"username":"u","password":"protected:other-machine"}`), 0o600)
	a := NewApp(dir)
	if a.cfg.Password != "" || a.cfg.Username != "u" {
		t.Fatalf("got %+v", a.cfg)
	}
}
