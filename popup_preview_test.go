package main

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPopupPreview(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("set PREVIEW_DIR")
	}
	c := NewCanvas(popW, popH)
	f := DrawPopup(c, sampleSnapshot(), &PopupState{Hover: "show"})
	img := image.NewRGBA(image.Rect(0, 0, popW, popH))
	copy(img.Pix, c.Pix)
	fh, _ := os.Create(filepath.Join(dir, "popup.png"))
	png.Encode(fh, img)
	fh.Close()
	b, _ := json.Marshal(f.Texts)
	os.WriteFile(filepath.Join(dir, "popup.json"), b, 0o644)
}
