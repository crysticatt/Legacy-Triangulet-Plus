package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	"sync"
)

//go:embed triangulet-logo.png
var logoPNG []byte

var (
	logoOnce sync.Once
	logoImg  *image.RGBA
)

// appLogo is the Triangulet logo scaled to 128x128 (premultiplied RGBA).
func appLogo() *image.RGBA {
	logoOnce.Do(func() {
		img, _, err := image.Decode(bytes.NewReader(logoPNG))
		if err != nil {
			logoImg = defaultTrianImage()
			return
		}
		logoImg = scaleToRGBA(img, 128, 128)
		_ = draw.Draw
	})
	return logoImg
}
