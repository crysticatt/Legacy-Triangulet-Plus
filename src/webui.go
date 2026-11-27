//go:build !windows

package main

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The Mac (and Linux) build has no native window. It runs the opener in the background and
// serves a dashboard on 127.0.0.1 that you open in the browser.

//go:embed webui.html
var webPageRaw []byte

// webPage is the dashboard with the list of every trian filled in.
var webPage = func() []byte {
	type tr struct {
		N string `json:"n"`
		R string `json:"r"`
		C string `json:"c"`
	}
	var l []tr
	for _, t := range builtinTrians {
		l = append(l, tr{t.Name, t.Rarity, t.Capsule})
	}
	j, _ := json.Marshal(l)
	return bytes.Replace(webPageRaw, []byte("__TRIANS__"), j, 1)
}()

type webServer struct {
	hub      *Hub
	port     int
	mu       sync.Mutex
	loginItm func() bool // is "run at login" on
	setLogin func(on bool) error
	quit     func()                    // stops the whole program
	applyUpd func(newExe string) error // swaps in a downloaded update and restarts
}

// allowedHost stops other websites from reaching the dashboard through DNS rebinding.
func (w *webServer) allowedHost(h string) bool {
	h = strings.ToLower(h)
	p := strconv.Itoa(w.port)
	return h == "127.0.0.1:"+p || h == "localhost:"+p
}

func (w *webServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Header().Set("Cache-Control", "no-store")
		_, _ = rw.Write(webPage)
	})
	mux.HandleFunc("/api/ping", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, map[string]any{"app": "TrianguletOpener", "version": appVersion})
	})
	mux.HandleFunc("/api/state", func(rw http.ResponseWriter, r *http.Request) {
		writeJSON(rw, w.state())
	})
	mux.HandleFunc("/api/log.csv", func(rw http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(w.hub.logPath())
		if err != nil {
			b = []byte("time,trian,rarity,capsule,new\n")
		}
		rw.Header().Set("Content-Type", "text/csv")
		rw.Header().Set("Content-Disposition", `attachment; filename="pull-log.csv"`)
		_, _ = rw.Write(b)
	})
	mux.HandleFunc("/api/do", w.doAction)
	mux.HandleFunc("/api/backup/export", w.backupExport)
	mux.HandleFunc("/api/backup/import", w.backupImport)
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !w.allowedHost(r.Host) {
			http.Error(rw, "forbidden", http.StatusForbidden)
			return
		}
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(rw, r)
	})
}

func writeJSON(rw http.ResponseWriter, v any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(rw).Encode(v)
}

type webState struct {
	Version     string
	Snap        Snapshot
	Username    string
	HasPassword bool
	Capsules    []string
	DiscordOn   bool
	LoginItem   bool
	CanLogin    bool
}

// state is what the page shows. The password never leaves this process.
func (w *webServer) state() webState {
	s := w.hub.Snapshot()
	c := w.hub.cfg()
	st := webState{Version: appVersion, Username: c.Username, HasPassword: c.Password != "", DiscordOn: c.DiscordOn}
	s.Password, s.Username = "", ""
	st.Capsules = s.Capsules
	if len(st.Capsules) == 0 {
		st.Capsules = builtinCapsules
	}
	if len(s.Inv) > 600 {
		s.Inv = s.Inv[:600]
	}
	st.Snap = s
	if w.loginItm != nil {
		st.LoginItem, st.CanLogin = w.loginItm(), w.setLogin != nil
	}
	return st
}

type doReq struct {
	A      string  `json:"a"`
	S      string  `json:"s"`
	P      string  `json:"p"`
	N      int     `json:"n"`
	F      float64 `json:"f"`
	On     bool    `json:"on"`
	Amount int     `json:"amount"`
	Bid    int     `json:"bid"`
	Pay    int     `json:"pay"`
	Bg     string  `json:"bg"`
	Card   string  `json:"card"`
	Accent string  `json:"accent"`
	Text   string  `json:"text"`
}

// guard lets only this dashboard's own page act: a page on another site can't send the header
// without a CORS check we never grant.
func guard(rw http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost || r.Header.Get("X-Opener") != "1" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		rw.WriteHeader(403)
		writeJSON(rw, map[string]any{"error": "forbidden"})
		return false
	}
	return true
}

func (w *webServer) backupExport(rw http.ResponseWriter, r *http.Request) {
	if !guard(rw, r) {
		return
	}
	var q struct {
		Pass string `json:"pass"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&q) != nil {
		rw.WriteHeader(400)
		writeJSON(rw, map[string]any{"error": "bad request"})
		return
	}
	b, err := w.hub.ExportBackup(q.Pass)
	if err != nil {
		rw.WriteHeader(400)
		writeJSON(rw, map[string]any{"error": err.Error()})
		return
	}
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Disposition", `attachment; filename="`+backupFileName()+`"`)
	_, _ = rw.Write(b)
}

func (w *webServer) backupImport(rw http.ResponseWriter, r *http.Request) {
	if !guard(rw, r) {
		return
	}
	var q struct {
		Pass string `json:"pass"`
		Data string `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 24<<20)).Decode(&q) != nil {
		rw.WriteHeader(400)
		writeJSON(rw, map[string]any{"error": "bad request"})
		return
	}
	raw, err := base64.StdEncoding.DecodeString(q.Data)
	if err == nil {
		err = w.hub.ImportBackup(raw, q.Pass)
	}
	if err != nil {
		rw.WriteHeader(400)
		writeJSON(rw, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(rw, map[string]any{"ok": true, "msg": "Restored from the backup."})
}

func (w *webServer) doAction(rw http.ResponseWriter, r *http.Request) {
	note := ""
	reply := func(err error) {
		if err != nil {
			rw.WriteHeader(400)
			writeJSON(rw, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(rw, map[string]any{"ok": true, "msg": note})
	}
	if !guard(rw, r) {
		return
	}
	var q doReq
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&q); err != nil {
		reply(errors.New("bad request"))
		return
	}
	h := w.hub
	c := h.cfg()
	switch q.A {
	case "start":
		s := h.Snapshot()
		if !s.HasToken && !s.HasCreds {
			reply(errors.New("Log in first: enter your username and password and press Save and log in."))
			return
		}
		if c.Mode != "hunt" && s.Capsule == "" {
			reply(errors.New("Pick a capsule first (or switch Mode to Hunt)."))
			return
		}
		h.Start()
	case "stop":
		h.Stop()
	case "login":
		user, pass := strings.TrimSpace(q.S), q.P
		if pass == "" {
			for _, a := range c.Accounts {
				if strings.EqualFold(a.Name, user) {
					pass = a.Password
				}
			}
			if pass == "" && strings.EqualFold(user, c.Username) {
				pass = c.Password
			}
		}
		if user == "" || pass == "" {
			reply(errors.New("Enter both a username and a password."))
			return
		}
		h.Login(user, pass)
	case "mode":
		if q.S != "fixed" && q.S != "advance" && q.S != "hunt" && q.S != "least" && q.S != "wish" {
			reply(errors.New("unknown mode"))
			return
		}
		h.SetMode(q.S)
	case "spread":
		h.SetSpread(q.N)
	case "workers":
		h.SetWorkers(q.N)
	case "inflight":
		h.SetInFlight(q.N)
	case "delay":
		h.SetDelay(q.F)
	case "capsule":
		h.SelectCapsule(q.S)
	case "goal":
		h.SetGoal(q.N)
	case "chase":
		h.SetChase(q.On)
	case "sound":
		h.SetSound(q.On)
	case "discord":
		h.update(func(c *Config) { c.DiscordOn = q.On })
	case "speedtest":
		h.MeasureLimits()
	case "checkupdate":
		h.CheckUpdateNow(true)
	case "autoupdate":
		h.SetAutoUpdate(q.On)
	case "updcheck":
		h.update(func(c *Config) { c.NoUpdateCheck = !q.On })
	case "hourly":
		h.update(func(c *Config) { c.HourlySummary = q.On })
	case "awake":
		h.update(func(c *Config) { c.KeepAwake = q.On })
	case "themesave":
		if err := h.SaveCustomTheme(CustomTheme{Name: q.S, Bg: q.Bg, Card: q.Card, Accent: q.Accent, Text: q.Text}); err != nil {
			reply(err)
			return
		}
		h.SetTheme("custom:" + strings.ToLower(cleanName(q.S)))
	case "themedel":
		h.DeleteCustomTheme(q.S)
	case "theme":
		h.SetTheme(q.S)
	case "ntfy":
		if m := h.SetNtfy(q.On, q.S); m != "" {
			reply(errors.New(m))
			return
		}
	case "wish":
		h.ToggleWish(q.S)
	case "diag":
		note = Diagnostics(h.Snapshot())
	case "downtime":
		if m := h.SetDowntime(q.S); m != "" {
			reply(errors.New(m))
			return
		}
	case "ntfyrand":
		h.RandomNtfyTopic()
	case "ntfytest":
		note = h.NtfyTest()
	case "whatsnew":
		h.DismissWhatsNew()
	case "update":
		if w.applyUpd == nil {
			reply(errors.New("not supported here"))
			return
		}
		h.InstallUpdate(w.applyUpd)
	case "refresh":
		h.Refresh()
	case "acct":
		h.SwitchAccount(q.N)
	case "acctremove":
		h.RemoveAccount()
	case "resetstats":
		h.ResetStats()
	case "auction":
		h.StartAuction(q.S, q.Amount, q.Bid, q.Pay)
	case "quit":
		if w.quit == nil {
			reply(errors.New("not supported here"))
			return
		}
		reply(nil)
		go func() { time.Sleep(300 * time.Millisecond); w.quit() }()
		return
	case "loginitem":
		if w.setLogin == nil {
			reply(errors.New("not supported here"))
			return
		}
		if err := w.setLogin(q.On); err != nil {
			reply(err)
			return
		}
	default:
		reply(fmt.Errorf("unknown action %q", q.A))
		return
	}
	reply(nil)
}

// listenDashboard binds 127.0.0.1 on the wanted port; if that is taken by another program
// it falls back to a free one. already is true when it is another copy of this app.
func listenDashboard(port int) (ln net.Listener, already bool, err error) {
	ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err == nil {
		return ln, false, nil
	}
	if pingDashboard(port) {
		return nil, true, nil
	}
	ln, err = net.Listen("tcp", "127.0.0.1:0")
	return ln, false, err
}

func pingDashboard(port int) bool {
	cl := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := cl.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/api/ping")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var v struct{ App string }
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return json.Unmarshal(b, &v) == nil && v.App == "TrianguletOpener"
}
