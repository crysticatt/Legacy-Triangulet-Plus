//go:build !windows

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newWebTest(t *testing.T) (*webServer, *httptest.Server, *fakeSite) {
	f := newFakeSite(100000)
	t.Cleanup(f.srv.Close)
	h := newTestHub(t, f)
	w := &webServer{hub: h, loginItm: func() bool { return false }}
	ts := httptest.NewServer(w.handler())
	t.Cleanup(ts.Close)
	w.port = ts.Listener.Addr().(*net.TCPAddr).Port
	return w, ts, f
}

func post(t *testing.T, ts *httptest.Server, body string, hdr map[string]string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+"/api/do", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Opener", "1")
	for k, v := range hdr {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestWebStateNeverContainsPassword(t *testing.T) {
	w, ts, _ := newWebTest(t)
	w.hub.update(func(c *Config) { c.Username = "alice"; c.Password = "s3cret-pass" })
	resp, err := http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(strings.Builder)
	var st map[string]any
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&st); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(st)
	buf.Write(b)
	if strings.Contains(buf.String(), "s3cret-pass") {
		t.Fatal("the password leaked into /api/state")
	}
	if st["Username"] != "alice" || st["HasPassword"] != true {
		t.Fatalf("state = %v", st["Username"])
	}
}

func TestWebRejectsForeignHostAndMissingHeader(t *testing.T) {
	_, ts, _ := newWebTest(t)
	req, _ := http.NewRequest("GET", ts.URL+"/api/state", nil)
	req.Host = "evil.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("foreign Host got %d", resp.StatusCode)
	}
	if code, _ := post(t, ts, `{"a":"stop"}`, map[string]string{"X-Opener": ""}); code != 403 {
		t.Fatalf("missing header got %d", code)
	}
	if code, _ := post(t, ts, `{"a":"stop"}`, map[string]string{"Content-Type": "text/plain"}); code != 403 {
		t.Fatalf("text/plain got %d", code)
	}
	if code, _ := post(t, ts, `{"a":"nope"}`, nil); code != 400 {
		t.Fatalf("unknown action got %d", code)
	}
	if code, out := post(t, ts, `{"a":"stop"}`, nil); code != 200 || out["ok"] != true {
		t.Fatalf("stop got %d %v", code, out)
	}
}

func TestWebActionsChangeSettings(t *testing.T) {
	w, ts, _ := newWebTest(t)
	for _, b := range []string{`{"a":"workers","n":4}`, `{"a":"inflight","n":5}`, `{"a":"mode","s":"hunt"}`, `{"a":"spread","n":2}`, `{"a":"delay","f":0.5}`, `{"a":"goal","n":3}`, `{"a":"chase","on":false}`, `{"a":"discord","on":false}`} {
		if code, out := post(t, ts, b, nil); code != 200 {
			t.Fatalf("%s -> %d %v", b, code, out)
		}
	}
	c := w.hub.cfg()
	if c.Workers != 4 || c.InFlight != 5 || c.Mode != "hunt" || c.HuntSpread != 2 || c.DelayMs != 0.5 || c.StopAfterMystical != 3 || c.HuntMissing || c.DiscordOn {
		t.Fatalf("cfg = %+v", c)
	}
	if code, _ := post(t, ts, `{"a":"mode","s":"bogus"}`, nil); code != 400 {
		t.Fatal("a bogus mode must be refused")
	}
}

func TestWebLoginStartStopAndSavedPassword(t *testing.T) {
	w, ts, f := newWebTest(t)
	w.hub.update(func(c *Config) { c.Capsule = "OG"; c.Mode = "fixed"; c.Username, c.Password = "", "" })
	if code, _ := post(t, ts, `{"a":"start"}`, nil); code != 400 {
		t.Fatal("start without a login must be refused")
	}
	if code, _ := post(t, ts, `{"a":"login","s":"alice","p":"pw1"}`, nil); code != 200 {
		t.Fatal("login refused")
	}
	waitFor(t, 5*time.Second, "token", func() bool { return w.hub.primary.hasToken() })
	post(t, ts, `{"a":"login","s":"bob","p":"pw2"}`, nil)
	waitFor(t, 5*time.Second, "bob", func() bool { return w.hub.cfg().Username == "bob" })
	// switching back with an empty password uses the saved one
	if code, _ := post(t, ts, `{"a":"login","s":"alice","p":""}`, nil); code != 200 {
		t.Fatal("saved password not used")
	}
	waitFor(t, 5*time.Second, "alice again", func() bool { c := w.hub.cfg(); return c.Username == "alice" && c.Password == "pw1" })
	waitFor(t, 5*time.Second, "token again", func() bool { return w.hub.primary.hasToken() })
	if code, _ := post(t, ts, `{"a":"start"}`, nil); code != 200 {
		t.Fatal("start refused")
	}
	waitFor(t, 5*time.Second, "opening", func() bool { return atomicLoad(&f.opens) > 3 })
	post(t, ts, `{"a":"stop"}`, nil)
}

func TestWebQuitCallsQuit(t *testing.T) {
	w, ts, _ := newWebTest(t)
	done := make(chan struct{}, 1)
	w.quit = func() { done <- struct{}{} }
	if code, _ := post(t, ts, `{"a":"quit"}`, nil); code != 200 {
		t.Fatal("quit refused")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("quit was not called")
	}
}

func TestWebPageAndCSV(t *testing.T) {
	w, ts, _ := newWebTest(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("page: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	r2, err := http.Get(ts.URL + "/api/log.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != 200 {
		t.Fatal("csv")
	}
	if r3, err := http.Get(ts.URL + "/nothing"); err != nil || r3.StatusCode != 404 {
		t.Fatal("404 expected")
	}
	_ = w
}

func TestListenDashboardFallsBackAndDetectsCopy(t *testing.T) {
	w, _, _ := newWebTest(t)
	// a port held by something that is not this app: fall back to another one
	other, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	p := other.Addr().(*net.TCPAddr).Port
	ln, already, err := listenDashboard(p)
	if err != nil || already || ln == nil {
		t.Fatalf("fallback: %v %v", already, err)
	}
	ln.Close()
	// a port held by this app: already running
	real, _ := net.Listen("tcp", "127.0.0.1:0")
	rp := real.Addr().(*net.TCPAddr).Port
	w.port = rp
	srv := &http.Server{Handler: w.handler()}
	go srv.Serve(real)
	defer srv.Close()
	if _, already, _ := listenDashboard(rp); !already {
		t.Fatal("did not notice the running copy")
	}
	_ = strconv.Itoa
}

func TestLaunchPlistEscapesAndPointsAtApp(t *testing.T) {
	p := launchPlist("/Applications/Tri & Co/Triangulet<Opener", "/Users/a b/Library/x")
	for _, want := range []string{"Tri &amp; Co", "Triangulet&lt;Opener", "<string>--background</string>", "<key>RunAtLoad</key><true/>", "xyz.triangulet.opener"} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q", want)
		}
	}
}

func TestLoginItemFileRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TRIOPENER_DIR", t.TempDir())
	if loginItemOn() {
		t.Fatal("should start off")
	}
	if err := setLoginItem(true); err != nil || !loginItemOn() {
		t.Fatalf("enable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")); err != nil {
		t.Fatal(err)
	}
	if err := setLoginItem(false); err != nil || loginItemOn() {
		t.Fatalf("disable: %v", err)
	}
}

func atomicLoad(p *int64) int64 { return atomic.LoadInt64(p) }

// PREVIEW_DIR=/dir go test -run TestWebPreviewState writes the sample state the dashboard shows.
func TestWebPreviewState(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR")
	}
	s := sampleSnapshot()
	s.Hist = rebaseHist(sampleHist())
	st := webState{Version: appVersion, Snap: s, Username: "crysticat", HasPassword: true, Capsules: builtinCapsules, DiscordOn: true, LoginItem: true, CanLogin: true}
	b, _ := json.Marshal(st)
	_ = os.WriteFile(filepath.Join(dir, "web-state.json"), b, 0o644)
}

// rebaseHist moves the sample history so it ends now (the page draws relative to today).
func rebaseHist(h []HistBucket) []HistBucket {
	if len(h) == 0 {
		return h
	}
	shift := time.Now().Truncate(time.Hour).Unix() - h[len(h)-1].T
	out := make([]HistBucket, len(h))
	for i, b := range h {
		b.T += shift
		out[i] = b
	}
	return out
}
