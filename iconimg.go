package main

import (
	"image"
	"math"
)

// scaleToRGBA resizes any image to w x h (premultiplied RGBA) with a box filter.
func scaleToRGBA(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return dst
	}
	for j := 0; j < h; j++ {
		y0 := j * sh / h
		y1 := maxi(y0+1, (j+1)*sh/h)
		for i := 0; i < w; i++ {
			x0 := i * sw / w
			x1 := maxi(x0+1, (i+1)*sw/w)
			var r, g, bl, a, n uint64
			for y := y0; y < y1 && y < sh; y++ {
				for x := x0; x < x1 && x < sw; x++ {
					pr, pg, pb, pa := src.At(b.Min.X+x, b.Min.Y+y).RGBA() // premultiplied, 16-bit
					r, g, bl, a, n = r+uint64(pr), g+uint64(pg), bl+uint64(pb), a+uint64(pa), n+1
				}
			}
			if n == 0 {
				continue
			}
			o := dst.PixOffset(i, j)
			dst.Pix[o] = uint8(r / n >> 8)
			dst.Pix[o+1] = uint8(g / n >> 8)
			dst.Pix[o+2] = uint8(bl / n >> 8)
			dst.Pix[o+3] = uint8(a / n >> 8)
		}
	}
	return dst
}

// triangleImage draws a glossy triangle on a transparent background.
func triangleImage(top, bot Col, size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	a := pt{s * 0.5, s * 0.10}
	b := pt{s * 0.08, s * 0.88}
	d := pt{s * 0.92, s * 0.88}
	outer := triInside(a, b, d)
	ia := pt{s * 0.5, s * 0.34}
	ib := pt{s * 0.27, s * 0.76}
	id := pt{s * 0.73, s * 0.76}
	inner := triInside(ia, ib, id)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			cov := clampf(0.5+outer(fx, fy), 0, 1)
			if cov <= 0 {
				continue
			}
			col := lerpCol(top, bot, (fy-s*0.10)/(s*0.78))
			// lighter inner triangle gives it depth
			if in := clampf(0.5+inner(fx, fy), 0, 1); in > 0 {
				hl := lerpCol(col, RGB(255, 255, 255), 0.35)
				col = lerpCol(col, hl, in)
			}
			o := img.PixOffset(x, y)
			img.Pix[o] = uint8(float64(col.R) * cov)
			img.Pix[o+1] = uint8(float64(col.G) * cov)
			img.Pix[o+2] = uint8(float64(col.B) * cov)
			img.Pix[o+3] = uint8(255 * cov)
		}
	}
	return img
}

// defaultTrianImage is the blue triangle shown until a Chroma has been pulled.
func defaultTrianImage() *image.RGBA {
	return triangleImage(RGB(86, 190, 255), RGB(30, 90, 214), 64)
}

// chromaPlaceholderImage is a cyan triangle used when a Chroma's picture can't be fetched.
func chromaPlaceholderImage() *image.RGBA {
	return triangleImage(RGB(120, 255, 244), RGB(0, 168, 196), 64)
}

func overStraightBGRA(p []byte, i int, c Col, cov float64) {
	sa := float64(c.A) / 255 * cov
	if sa <= 0 {
		return
	}
	da := float64(p[i+3]) / 255
	oa := sa + da*(1-sa)
	if oa <= 0 {
		return
	}
	mix := func(dst byte, src uint8) byte {
		return byte(clampf((float64(src)*sa+float64(dst)*da*(1-sa))/oa, 0, 255) + 0.5)
	}
	p[i], p[i+1], p[i+2] = mix(p[i], c.B), mix(p[i+1], c.G), mix(p[i+2], c.R)
	p[i+3] = byte(clampf(oa*255, 0, 255) + 0.5)
}

// iconPixels renders the 32x32 tray icon (BGRA, straight alpha): the picture, plus a
// green dot in the corner while the opener is running.
func iconPixels(img *image.RGBA, running bool) []byte {
	const n = 32
	if img == nil {
		img = defaultTrianImage()
	}
	small := scaleToRGBA(img, n, n)
	out := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			i := (y*n + x) * 4
			s := small.PixOffset(x, y)
			a := uint32(small.Pix[s+3])
			if a == 0 {
				continue
			}
			out[i] = byte(uint32(small.Pix[s+2]) * 255 / a)
			out[i+1] = byte(uint32(small.Pix[s+1]) * 255 / a)
			out[i+2] = byte(uint32(small.Pix[s]) * 255 / a)
			out[i+3] = byte(a)
		}
	}
	if running {
		cx, cy := 24.5, 24.5
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
				i := (y*n + x) * 4
				overStraightBGRA(out, i, RGB(6, 18, 48), cov01(d-7.4))       // dark ring
				overStraightBGRA(out, i, RGB(60, 230, 130), cov01(d-5.4))    // green dot
				overStraightBGRA(out, i, RGB(190, 255, 215), 0.5*cov01(d-2)) // highlight
			}
		}
	}
	return out
}
