package main

import (
	"image"
	"sync"
)

// Pictures for the trians list are fetched on demand, one at a time in the background,
// and kept in memory. Until a picture arrives (or if it never does) a coloured triangle is shown.

var (
	imgMu      sync.Mutex
	imgCache   = map[string]*image.RGBA{}
	imgPending = map[string]bool{}
	imgMisses  = map[string]bool{}
	imgSlots   = make(chan struct{}, 3)
	imgHub     *Hub
)

// trianImage returns the picture for a trian name, or nil while it is loading.
func trianImage(name string) *image.RGBA {
	imgMu.Lock()
	defer imgMu.Unlock()
	if im, ok := imgCache[name]; ok {
		return im
	}
	if imgMisses[name] || imgPending[name] || imgHub == nil {
		return nil
	}
	imgPending[name] = true
	go fetchTrianImage(name)
	return nil
}

func fetchTrianImage(name string) {
	imgSlots <- struct{}{}
	defer func() { <-imgSlots }()
	h := imgHub
	cap, _ := capsuleOf(name)
	p := h.primary
	p.mu.Lock()
	base := p.cfg.BaseURL
	var hint string
	for k, v := range p.hints {
		if i := indexByte(k, 0); i >= 0 && k[i+1:] == name {
			hint = v
			break
		}
	}
	p.mu.Unlock()
	var found *image.RGBA
	if cap != "" {
		for _, u := range trianImageURLs(base, cap, name, hint) {
			if _, im, ok := p.tryImage(u); ok {
				found = scaleToRGBA(im, 56, 56)
				break
			}
		}
	}
	imgMu.Lock()
	delete(imgPending, name)
	if found != nil {
		imgCache[name] = found
	} else {
		imgMisses[name] = true
	}
	imgMu.Unlock()
	if found != nil {
		imgDirty()
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// imgDirty is set by the window code so a newly arrived picture is drawn.
var imgDirty = func() {}
