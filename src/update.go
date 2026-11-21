package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultUpdateRepo = "crysticatt/crysticat-triangulet-opener"
	updateEvery       = 6 * time.Hour
	maxUpdateSize     = 64 << 20
)

// updateAPI and updateStrict are variables so tests can point the updater at a fake GitHub.
// assetMatch and looksExecutable depend on the system (see update_plat_*.go).
var (
	updateAPI    = "https://api.github.com"
	updateStrict = true // downloads must come from GitHub's own hosts
)

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"` // "sha256:<hex>", added by GitHub for uploaded files
	Size   int64  `json:"size"`
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	Name       string    `json:"name"`
	Body       string    `json:"body"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

// UpdateInfo describes a newer release that can be installed.
type UpdateInfo struct {
	Version string // "36"
	Tag     string
	Notes   string
	ExeURL  string
	SumURL  string
	Digest  string // expected sha256 (hex) when GitHub supplied it
	Size    int64
}

// parseVer turns "v1.2.10" or "35" into numbers to compare.
func parseVer(s string) []int {
	s = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "vV"))
	var out []int
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

// versionNewer reports whether a is a higher version than b.
func versionNewer(a, b string) bool {
	x, y := parseVer(a), parseVer(b)
	if len(x) == 0 {
		return false
	}
	for i := 0; i < len(x) || i < len(y); i++ {
		var p, q int
		if i < len(x) {
			p = x[i]
		}
		if i < len(y) {
			q = y[i]
		}
		if p != q {
			return p > q
		}
	}
	return false
}

func allowedUpdateHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || h == "api.github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

func (a *App) updateRepo() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := strings.TrimSpace(a.cfg.UpdateRepo)
	if r == "" {
		r = defaultUpdateRepo
	}
	if !repoRe.MatchString(r) {
		return ""
	}
	return r
}

// CheckUpdate asks GitHub for the latest release. It returns nil when there is nothing newer
// (or the release has no file for this system).
func (a *App) CheckUpdate() (*UpdateInfo, error) {
	repo := a.updateRepo()
	if repo == "" {
		return nil, errors.New("updateRepo in config.json is not a valid owner/repo")
	}
	req, err := http.NewRequest("GET", updateAPI+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "TrianguletOpener/"+appVersion)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, nil // no release published yet
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub answered HTTP %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.Draft || rel.Prerelease || !versionNewer(rel.Tag, appVersion) {
		return nil, nil
	}
	info := &UpdateInfo{Tag: rel.Tag, Notes: rel.Body}
	if v := parseVer(rel.Tag); len(v) > 0 {
		parts := make([]string, len(v))
		for i, n := range v {
			parts[i] = strconv.Itoa(n)
		}
		info.Version = strings.Join(parts, ".")
	}
	for _, as := range rel.Assets {
		if assetMatch(as.Name) {
			info.ExeURL, info.Size = as.URL, as.Size
			if d := strings.ToLower(as.Digest); strings.HasPrefix(d, "sha256:") && len(d) == 71 {
				info.Digest = d[7:]
			}
		}
	}
	for _, as := range rel.Assets {
		n := strings.ToLower(as.Name)
		if info.ExeURL != "" && (n == strings.ToLower(pathBase(info.ExeURL))+".sha256" || n == "sha256sums" || n == "sha256sums.txt") {
			info.SumURL = as.URL
		}
	}
	if info.ExeURL == "" {
		return nil, nil // the release has no file for this system (yet)
	}
	return info, nil
}

func pathBase(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return filepath.Base(u.Path)
	}
	return filepath.Base(raw)
}

var hexRe = regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`)

func (a *App) fetchLimited(raw string, limit int64) ([]byte, error) {
	if updateStrict && !allowedUpdateHost(raw) {
		return nil, errors.New("refusing to download from " + raw)
	}
	req, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TrianguletOpener/"+appVersion)
	client := &http.Client{Transport: a.client.Transport, Timeout: 3 * time.Minute}
	if updateStrict {
		client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
			if len(via) > 5 || !allowedUpdateHost(r.URL.String()) {
				return errors.New("redirected to a host that is not GitHub")
			}
			return nil
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download answered HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("download is too large")
	}
	return b, nil
}

// DownloadUpdate fetches the program, checks it and saves it to dest. When the release has a
// checksum (GitHub adds one to uploaded files) it must match; otherwise the download must at
// least be the size GitHub lists.
func (a *App) DownloadUpdate(info *UpdateInfo, dest string) error {
	want := info.Digest
	if want == "" && info.SumURL != "" {
		b, err := a.fetchLimited(info.SumURL, 1<<20)
		if err != nil {
			return fmt.Errorf("could not read the checksum: %v", err)
		}
		want = strings.ToLower(hexRe.FindString(string(b)))
		base := strings.ToLower(pathBase(info.ExeURL))
		for _, line := range strings.Split(string(b), "\n") { // a SHA256SUMS file: pick this file's line
			if strings.Contains(strings.ToLower(line), base) {
				if m := hexRe.FindString(line); m != "" {
					want = strings.ToLower(m)
				}
			}
		}
	}
	if len(want) != 64 {
		want = ""
	}
	exe, err := a.fetchLimited(info.ExeURL, maxUpdateSize)
	if err != nil {
		return err
	}
	switch {
	case want != "":
		sum := sha256.Sum256(exe)
		if hex.EncodeToString(sum[:]) != want {
			return errors.New("the download does not match its checksum, so it was not installed")
		}
	case info.Size > 0:
		if int64(len(exe)) != info.Size {
			return errors.New("the download is incomplete, so it was not installed")
		}
	default:
		return errors.New("the release has no checksum or size, so it was not installed")
	}
	if len(exe) < 500_000 || !looksExecutable(exe) {
		return errors.New("the download is not a program for this system")
	}
	return os.WriteFile(dest, exe, 0o755)
}

// ---------- hub side ----------

// CheckUpdateNow runs a check in the background and keeps the result for the window.
// manual is true when the person pressed "Check for updates" (so "up to date" is reported).
func (h *Hub) CheckUpdateNow(manual bool) {
	h.mu.Lock()
	if h.updBusy {
		h.mu.Unlock()
		return
	}
	h.updBusy = true
	h.updTried = time.Now()
	if manual {
		h.updNote = "Checking..."
	}
	h.mu.Unlock()
	h.bg(func() {
		info, err := h.primary.CheckUpdate()
		h.mu.Lock()
		h.updBusy = false
		switch {
		case err != nil:
			h.primary.logf("update check: %v", err)
			if manual {
				h.updNote = "Could not check: " + err.Error()
			}
		default:
			h.upd = info
			if manual {
				h.updNote = "You have the latest version (v" + appVersion + ")"
				if info != nil {
					h.updNote = "Version " + info.Version + " is available"
				}
			}
		}
		auto := err == nil && info != nil && h.applyUpd != nil && h.cfg().AutoUpdate && (h.canAuto == nil || h.canAuto())
		apply := h.applyUpd
		h.mu.Unlock()
		if auto {
			h.primary.logf("installing update v%s automatically", info.Version)
			h.InstallUpdate(apply)
		}
	})
}

// maybeCheckUpdate is called from the supervisor: once soon after start, then every few hours.
func (h *Hub) maybeCheckUpdate() {
	if h.cfg().NoUpdateCheck {
		return
	}
	h.mu.Lock()
	due := (h.updTried.IsZero() && time.Since(h.bootAt) > 8*time.Second) || (!h.updTried.IsZero() && time.Since(h.updTried) > updateEvery)
	pending := h.upd != nil && !h.updBusy && h.updMsg == "" && h.applyUpd != nil && h.cfg().AutoUpdate && (h.canAuto == nil || h.canAuto())
	apply := h.applyUpd
	h.mu.Unlock()
	if pending { // an update found earlier while the window was open: install once it is hidden
		h.InstallUpdate(apply)
	} else if due {
		h.CheckUpdateNow(false)
	}
}

// DismissWhatsNew hides the release notes shown after an update.
func (h *Hub) DismissWhatsNew() {
	h.update(func(c *Config) { c.WhatsNewVer, c.WhatsNewNotes = "", "" })
}

// SetAutoUpdate turns installing updates by itself on or off.
func (h *Hub) SetAutoUpdate(on bool) { h.update(func(c *Config) { c.AutoUpdate = on }) }

// InstallUpdate downloads the pending update and hands the new file to apply (which swaps the
// program and restarts it). apply is system-specific.
func (h *Hub) InstallUpdate(apply func(newExe string) error) {
	h.mu.Lock()
	info := h.upd
	if info == nil || h.updBusy {
		h.mu.Unlock()
		return
	}
	h.updBusy = true
	h.updMsg = "Downloading..."
	h.mu.Unlock()
	h.bg(func() {
		h.primary.logf("update: installing version %s", info.Version)
		set := func(msg string) {
			if msg != "" {
				h.primary.logf("update: %s", msg)
			}
			h.mu.Lock()
			h.updMsg, h.updBusy = msg, false
			h.mu.Unlock()
		}
		dest := filepath.Join(h.primary.dir, "update-new")
		if err := h.primary.DownloadUpdate(info, dest); err != nil {
			_ = os.Remove(dest)
			set("Update failed: " + err.Error())
			return
		}
		h.mu.Lock()
		h.updMsg = "Installing..."
		h.mu.Unlock()
		// show the release notes once the new version starts
		ver := strings.Join(strings.FieldsFunc(info.Version, func(r rune) bool { return r == '.' }), ".")
		h.update(func(c *Config) { c.WhatsNewVer, c.WhatsNewNotes = ver, strings.TrimSpace(info.Notes) })
		if err := apply(dest); err != nil {
			_ = os.Remove(dest)
			h.update(func(c *Config) { c.WhatsNewVer, c.WhatsNewNotes = "", "" })
			set("Update failed: " + err.Error())
			return
		}
		set("")
	})
}
