package main

import (
	"bytes"
	"encoding/json"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The tray icon shows the rarest Chroma you have pulled. "Rarest" means the lowest
// drop chance listed for it in the capsule data. It is kept across runs.

type bestChroma struct {
	Name    string  `json:"name"`
	Capsule string  `json:"capsule"`
	Chance  float64 `json:"chance"`           // percent; lower is rarer; 100 = unknown
	Rarity  string  `json:"rarity,omitempty"` // "Chroma" or "Mystical" (a Mystical always beats a Chroma)
}

func rarityScore(r string) int {
	if r == "Mystical" {
		return 1
	}
	return 0
}

func chanceKey(capsule, name string) string { return capsule + "\x00" + name }

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "%"))
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	return 0, false
}

func (a *App) loadBest() {
	b, err := os.ReadFile(filepath.Join(a.dir, "best.json"))
	if err != nil {
		return
	}
	var bc bestChroma
	if json.Unmarshal(b, &bc) != nil || bc.Name == "" {
		return
	}
	a.best = &bc
	if data, err := os.ReadFile(filepath.Join(a.dir, "chroma.png")); err == nil {
		if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
			a.bestImg = scaleToRGBA(img, 64, 64)
			a.iconVer = 1
		}
	}
}

// ChromaImage returns the picture for the rarest Chroma (nil until one is pulled).
func (a *App) ChromaImage() (*image.RGBA, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.bestImg, a.iconVer
}

// LoadChromaIfMissing fetches the picture at startup if only the name was saved.
func (a *App) LoadChromaIfMissing() {
	a.mu.Lock()
	need := a.best != nil && a.bestImg == nil
	a.mu.Unlock()
	if need {
		a.loadBestImage()
	}
}

func (a *App) considerChroma(capsule, name string) { a.considerRare(capsule, name, "Chroma") }

// considerRare keeps the rarest pull for the tray icon: any Mystical beats every Chroma,
// and within a kind the lower drop chance wins.
func (a *App) considerRare(capsule, name, rarity string) {
	a.mu.Lock()
	ch, ok := a.chances[chanceKey(capsule, name)]
	if !ok {
		ch = 100
	}
	if a.best != nil {
		if a.best.Name == name && a.best.Capsule == capsule {
			if ch < a.best.Chance { // learned its real chance
				a.best.Chance = ch
				a.saveBestLocked()
			}
			a.mu.Unlock()
			return
		}
		old := rarityScore(a.best.Rarity)
		if rarityScore(rarity) < old || (rarityScore(rarity) == old && ch >= a.best.Chance) {
			a.mu.Unlock() // not rarer than what we already have
			return
		}
	}
	a.best = &bestChroma{Name: name, Capsule: capsule, Chance: ch, Rarity: rarity}
	a.saveBestLocked()
	a.mu.Unlock()
	go a.loadBestImage()
}

func (a *App) saveBestLocked() {
	if a.best == nil {
		return
	}
	if b, err := json.Marshal(a.best); err == nil {
		_ = os.WriteFile(filepath.Join(a.dir, "best.json"), b, 0o600)
	}
}

func uniqStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// trianImageURLs lists the likely addresses of a trian's picture. File names on the
// site are not consistent (e.g. "Monkey" is monkey.png), so a few spellings are tried.
func trianImageURLs(base, capsule, name, hint string) []string {
	var out []string
	if hint != "" {
		if strings.HasPrefix(hint, "http") {
			out = append(out, hint)
		} else {
			out = append(out, base+"/"+strings.TrimLeft(hint, "/"))
		}
	}
	lower := strings.ToLower(name)
	caps := uniqStrings([]string{capsule, strings.ToLower(capsule)})
	names := uniqStrings([]string{name, lower, strings.ReplaceAll(lower, " ", ""), strings.ReplaceAll(lower, " ", "_"), strings.ReplaceAll(name, " ", "")})
	for _, c := range caps {
		for _, n := range names {
			out = append(out, base+"/media/capsules/"+url.PathEscape(c)+"/trians/"+url.PathEscape(n)+".png")
		}
	}
	return uniqStrings(out)
}

func (a *App) tryImage(u string) ([]byte, image.Image, bool) {
	resp, err := a.client.Get(u)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil, false
	}
	return data, img, true
}

func (a *App) loadBestImage() {
	a.mu.Lock()
	if a.best == nil {
		a.mu.Unlock()
		return
	}
	bc := *a.best
	base := a.cfg.BaseURL
	hint := a.hints[chanceKey(bc.Capsule, bc.Name)]
	a.mu.Unlock()

	var found image.Image
	var data []byte
	for _, u := range trianImageURLs(base, bc.Capsule, bc.Name, hint) {
		if d, im, ok := a.tryImage(u); ok {
			data, found = d, im
			break
		}
	}
	var small *image.RGBA
	cache := filepath.Join(a.dir, "chroma.png")
	if found != nil {
		small = scaleToRGBA(found, 64, 64)
		_ = os.WriteFile(cache, data, 0o600)
	} else {
		small = chromaPlaceholderImage()
		_ = os.Remove(cache)
		a.logf("no picture found for Chroma %q (%s)", bc.Name, bc.Capsule)
	}
	a.mu.Lock()
	if a.best != nil && a.best.Name == bc.Name && a.best.Capsule == bc.Capsule {
		a.bestImg = small
		a.iconVer++
	}
	a.mu.Unlock()
}

// Startup default: the app registers itself to run at Windows startup the first time.
func (a *App) StartupInitDone() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.StartupInit
}

func (a *App) MarkStartupInit() {
	a.mu.Lock()
	a.cfg.StartupInit = true
	a.saveConfigLocked()
	a.mu.Unlock()
}

var _ = http.StatusOK
