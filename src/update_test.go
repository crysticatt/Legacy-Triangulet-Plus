package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVersionNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"v36", "35", true}, {"v35", "35", false}, {"v34", "35", false}, {"v1.2.10", "1.2.9", true}, {"v10", "9", true}, {"junk", "1", false}} {
		if got := versionNewer(c.a, c.b); got != c.want {
			t.Errorf("versionNewer(%q,%q)=%v", c.a, c.b, got)
		}
	}
}

func fakeProgram() []byte {
	b := make([]byte, 600_000)
	if runtime.GOOS == "windows" {
		b[0], b[1] = 'M', 'Z'
	} else {
		copy(b, []byte{0xcf, 0xfa, 0xed, 0xfe})
	}
	return b
}

func assetFor() string {
	if runtime.GOOS == "windows" {
		return "TrianguletOpener-v99.exe"
	}
	return "TrianguletOpener-mac-" + runtime.GOARCH + "-v99"
}

func TestUpdateCheckAndDownload(t *testing.T) {
	prog := fakeProgram()
	sum := sha256.Sum256(prog)
	good := "sha256:" + hex.EncodeToString(sum[:])
	digest := good
	tag := "v99"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "body": "notes", "assets": []map[string]any{
				{"name": "TrianguletOpener-Setup-v99.exe", "browser_download_url": srv.URL + "/dl/setup", "size": 1},
				{"name": assetFor(), "browser_download_url": srv.URL + "/dl/prog", "size": len(prog), "digest": digest},
			}})
		case r.URL.Path == "/dl/prog":
			w.Write(prog)
		}
	}))
	defer srv.Close()
	oldAPI, oldStrict := updateAPI, updateStrict
	updateAPI, updateStrict = srv.URL, false
	defer func() { updateAPI, updateStrict = oldAPI, oldStrict }()

	h := newTestHub(t, newFakeSite(100))
	defer h.Close()
	info, err := h.primary.CheckUpdate()
	if err != nil || info == nil || info.Version != "99" || !strings.HasSuffix(info.ExeURL, "/dl/prog") {
		t.Fatalf("check: %+v %v", info, err)
	}
	dest := filepath.Join(t.TempDir(), "new")
	if err := h.primary.DownloadUpdate(info, dest); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(dest); st == nil || st.Size() != int64(len(prog)) {
		t.Fatal("file not saved")
	}
	// wrong checksum is refused
	info.Digest = strings.Repeat("0", 64)
	if err := h.primary.DownloadUpdate(info, filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("bad checksum accepted")
	}
	// the same version is not an update
	tag = "v" + appVersion
	if i, _ := h.primary.CheckUpdate(); i != nil {
		t.Fatal("same version offered")
	}
	// downloads from non-GitHub hosts are refused in strict mode
	updateStrict = true
	if _, err := h.primary.fetchLimited(srv.URL+"/dl/prog", 1<<20); err == nil {
		t.Fatal("non-GitHub host allowed")
	}
}

func TestInstallUpdateFlow(t *testing.T) {
	prog := fakeProgram()
	sum := sha256.Sum256(prog)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(prog) }))
	defer srv.Close()
	old := updateStrict
	updateStrict = false
	defer func() { updateStrict = old }()
	h := newTestHub(t, newFakeSite(100))
	defer h.Close()
	h.mu.Lock()
	h.upd = &UpdateInfo{Version: "99", ExeURL: srv.URL + "/p", Digest: hex.EncodeToString(sum[:])}
	h.mu.Unlock()
	got := make(chan string, 1)
	h.InstallUpdate(func(p string) error { got <- p; return nil })
	waitFor(t, 5*time.Second, "apply", func() bool { return len(got) == 1 })
	if b, err := os.ReadFile(<-got); err != nil || len(b) != len(prog) {
		t.Fatal("apply did not receive the file")
	}
}
