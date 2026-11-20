package main

import (
	"image"
	"math"
)

// A tiny anti-aliased software renderer. The window is drawn into a Canvas and
// copied to the screen in one go, so the look doesn't depend on Windows themes.

type Col struct{ R, G, B, A uint8 }

func RGB(r, g, b uint8) Col { return Col{r, g, b, 255} }

// A returns the colour with the given opacity (0..1).
func (c Col) Op(a float64) Col {
	c.A = uint8(clampf(a, 0, 1)*255 + 0.5)
	return c
}

func lerpCol(a, b Col, t float64) Col {
	t = clampf(t, 0, 1)
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return Col{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), f(a.A, b.A)}
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type Rect struct{ X, Y, W, H int }

func (r Rect) In(x, y int) bool { return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H }
func (r Rect) Inset(dx, dy int) Rect {
	return Rect{r.X + dx, r.Y + dy, r.W - 2*dx, r.H - 2*dy}
}
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := maxi(r.X, o.X), maxi(r.Y, o.Y)
	x1, y1 := mini(r.X+r.W, o.X+o.W), mini(r.Y+r.H, o.Y+o.H)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type Canvas struct {
	W, H int
	Pix  []uint8 // RGBA, treated as opaque
	clip Rect
}

func NewCanvas(w, h int) *Canvas {
	return &Canvas{W: w, H: h, Pix: make([]uint8, w*h*4), clip: Rect{0, 0, w, h}}
}

func (c *Canvas) CopyFrom(o *Canvas) { copy(c.Pix, o.Pix) }

func (c *Canvas) SetClip(r Rect) { c.clip = r.Intersect(Rect{0, 0, c.W, c.H}) }
func (c *Canvas) ResetClip()     { c.clip = Rect{0, 0, c.W, c.H} }

func (c *Canvas) blend(x, y int, col Col, cov float64) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	a := float64(col.A) / 255 * cov
	if a <= 0 {
		return
	}
	i := (y*c.W + x) * 4
	p := c.Pix[i : i+4 : i+4]
	if a >= 0.999 {
		p[0], p[1], p[2], p[3] = col.R, col.G, col.B, 255
		return
	}
	p[0] = uint8(float64(p[0])*(1-a) + float64(col.R)*a + 0.5)
	p[1] = uint8(float64(p[1])*(1-a) + float64(col.G)*a + 0.5)
	p[2] = uint8(float64(p[2])*(1-a) + float64(col.B)*a + 0.5)
	p[3] = 255
}

// paint evaluates coverage and colour per pixel inside b (clipped).
func (c *Canvas) paint(b Rect, cov func(x, y float64) float64, col func(x, y float64) Col) {
	b = b.Intersect(c.clip)
	for y := b.Y; y < b.Y+b.H; y++ {
		for x := b.X; x < b.X+b.W; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			cv := cov(fx, fy)
			if cv <= 0 {
				continue
			}
			c.blend(x, y, col(fx, fy), cv)
		}
	}
}

func solid(col Col) func(x, y float64) Col { return func(x, y float64) Col { return col } }

func cov01(d float64) float64 { return clampf(0.5-d, 0, 1) }

func sdfRound(px, py, x, y, w, h, r float64) float64 {
	r = math.Min(r, math.Min(w, h)/2)
	cx, cy := x+w/2, y+h/2
	qx := math.Abs(px-cx) - (w/2 - r)
	qy := math.Abs(py-cy) - (h/2 - r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

// fillRect fills a rectangle with integer maths (the fast path for flat areas).
func (c *Canvas) fillRect(r Rect, col Col) {
	r = r.Intersect(c.clip)
	if r.Empty() || col.A == 0 {
		return
	}
	a := int(col.A)
	for y := r.Y; y < r.Y+r.H; y++ {
		i := (y*c.W + r.X) * 4
		row := c.Pix[i : i+r.W*4]
		if a == 255 {
			for x := 0; x < len(row); x += 4 {
				row[x], row[x+1], row[x+2], row[x+3] = col.R, col.G, col.B, 255
			}
			continue
		}
		for x := 0; x < len(row); x += 4 {
			row[x] = uint8((int(row[x])*(255-a) + int(col.R)*a + 127) / 255)
			row[x+1] = uint8((int(row[x+1])*(255-a) + int(col.G)*a + 127) / 255)
			row[x+2] = uint8((int(row[x+2])*(255-a) + int(col.B)*a + 127) / 255)
			row[x+3] = 255
		}
	}
}

func (c *Canvas) Gradient(r Rect, top, bot Col) {
	if top == bot {
		c.fillRect(r, top)
		return
	}
	r = r.Intersect(c.clip)
	for y := r.Y; y < r.Y+r.H; y++ {
		t := float64(y-r.Y) / math.Max(1, float64(r.H-1))
		col := lerpCol(top, bot, t)
		for x := r.X; x < r.X+r.W; x++ {
			c.blend(x, y, col, 1)
		}
	}
}

func (c *Canvas) DiagGradient(r Rect, c1, c2 Col) {
	c.paint(r, func(x, y float64) float64 { return 1 }, func(x, y float64) Col {
		t := ((x-float64(r.X))/float64(r.W) + (y-float64(r.Y))/float64(r.H)) / 2
		return lerpCol(c1, c2, t)
	})
}

// Fill paints a plain rectangle (fast path, integer coverage).
func (c *Canvas) Fill(r Rect, col Col) {
	r = r.Intersect(c.clip)
	if r.Empty() || col.A == 0 {
		return
	}
	if col.A == 255 {
		for y := r.Y; y < r.Y+r.H; y++ {
			i := (y*c.W + r.X) * 4
			for x := 0; x < r.W; x++ {
				c.Pix[i], c.Pix[i+1], c.Pix[i+2], c.Pix[i+3] = col.R, col.G, col.B, 255
				i += 4
			}
		}
		return
	}
	a := uint32(col.A)
	ia := 255 - a
	for y := r.Y; y < r.Y+r.H; y++ {
		i := (y*c.W + r.X) * 4
		for x := 0; x < r.W; x++ {
			c.Pix[i] = uint8((uint32(c.Pix[i])*ia + uint32(col.R)*a + 127) / 255)
			c.Pix[i+1] = uint8((uint32(c.Pix[i+1])*ia + uint32(col.G)*a + 127) / 255)
			c.Pix[i+2] = uint8((uint32(c.Pix[i+2])*ia + uint32(col.B)*a + 127) / 255)
			c.Pix[i+3] = 255
			i += 4
		}
	}
}

// RoundRect fills a rounded rectangle. The straight middle is filled directly and
// only the four corners go through the anti-aliasing path, which keeps big cards cheap.
func (c *Canvas) RoundRect(r Rect, rad float64, fill Col) {
	fx, fy, fw, fh := float64(r.X), float64(r.Y), float64(r.W), float64(r.H)
	rad = math.Min(rad, math.Min(fw, fh)/2)
	ir := int(math.Ceil(rad))
	clip := c.clip
	for y := maxi(r.Y-1, clip.Y); y < mini(r.Y+r.H+1, clip.Y+clip.H); y++ {
		// the part of this row that is certainly fully covered
		fast0, fast1 := 0, 0
		switch {
		case y >= r.Y+ir && y < r.Y+r.H-ir:
			fast0, fast1 = r.X+1, r.X+r.W-1
		case y >= r.Y+1 && y < r.Y+r.H-1:
			fast0, fast1 = r.X+ir, r.X+r.W-ir
		}
		if fast1 > fast0 {
			c.fillRect(Rect{fast0, y, fast1 - fast0, 1}, fill)
		}
		for x := maxi(r.X-1, clip.X); x < mini(r.X+r.W+1, clip.X+clip.W); x++ {
			if x >= fast0 && x < fast1 {
				x = fast1 - 1
				continue
			}
			px, py := float64(x)+0.5, float64(y)+0.5
			if cv := cov01(sdfRound(px, py, fx, fy, fw, fh, rad)); cv > 0 {
				c.blend(x, y, fill, cv)
			}
		}
	}
}

func (c *Canvas) RoundRectGrad(r Rect, rad float64, top, bot Col) {
	fx, fy, fw, fh := float64(r.X), float64(r.Y), float64(r.W), float64(r.H)
	c.paint(Rect{r.X - 1, r.Y - 1, r.W + 2, r.H + 2},
		func(x, y float64) float64 { return cov01(sdfRound(x, y, fx, fy, fw, fh, rad)) },
		func(x, y float64) Col { return lerpCol(top, bot, (y-fy)/fh) })
}

func (c *Canvas) RoundRectStroke(r Rect, rad, width float64, col Col) {
	fx, fy, fw, fh := float64(r.X), float64(r.Y), float64(r.W), float64(r.H)
	rad = math.Min(rad, math.Min(fw, fh)/2)
	edge := int(math.Ceil(math.Max(rad, width+2))) + 1
	wi := int(math.Ceil(width)) + 2
	clip := c.clip
	cover := func(x, y int) {
		px, py := float64(x)+0.5, float64(y)+0.5
		d := sdfRound(px, py, fx, fy, fw, fh, rad)
		if cv := clampf(0.5-d, 0, 1) - clampf(0.5-(d+width), 0, 1); cv > 0 {
			c.blend(x, y, col, cv)
		}
	}
	for y := maxi(r.Y-1, clip.Y); y < mini(r.Y+r.H+1, clip.Y+clip.H); y++ {
		nearEdge := y < r.Y+wi || y >= r.Y+r.H-wi
		cornerRow := y < r.Y+edge || y >= r.Y+r.H-edge
		for x := maxi(r.X-1, clip.X); x < mini(r.X+r.W+1, clip.X+clip.W); x++ {
			if !nearEdge {
				spanL, spanR := wi, wi
				if cornerRow {
					spanL, spanR = edge, edge
				}
				if x >= r.X+spanL && x < r.X+r.W-spanR {
					x = r.X + r.W - spanR - 1
					continue
				}
			}
			cover(x, y)
		}
	}
}

func (c *Canvas) Circle(cx, cy, rad float64, fill Col) {
	b := Rect{int(cx - rad - 1), int(cy - rad - 1), int(2*rad + 3), int(2*rad + 3)}
	c.paint(b, func(x, y float64) float64 { return cov01(math.Hypot(x-cx, y-cy) - rad) }, solid(fill))
}

// Glow is a soft radial light.
func (c *Canvas) Glow(cx, cy, rad float64, col Col) {
	b := Rect{int(cx - rad), int(cy - rad), int(2*rad + 1), int(2*rad + 1)}
	c.paint(b, func(x, y float64) float64 {
		t := 1 - math.Hypot(x-cx, y-cy)/rad
		if t <= 0 {
			return 0
		}
		return t * t
	}, solid(col))
}

type pt [2]float64

func triInside(a, b, d pt) func(x, y float64) float64 {
	area := (b[0]-a[0])*(d[1]-a[1]) - (b[1]-a[1])*(d[0]-a[0])
	s := 1.0
	if area < 0 {
		s = -1
	}
	edge := func(p, q pt, x, y float64) float64 {
		l := math.Hypot(q[0]-p[0], q[1]-p[1])
		return s * ((q[0]-p[0])*(y-p[1]) - (q[1]-p[1])*(x-p[0])) / l
	}
	return func(x, y float64) float64 {
		return math.Min(edge(a, b, x, y), math.Min(edge(b, d, x, y), edge(d, a, x, y)))
	}
}

func triBounds(a, b, d pt) Rect {
	x0 := math.Min(a[0], math.Min(b[0], d[0]))
	y0 := math.Min(a[1], math.Min(b[1], d[1]))
	x1 := math.Max(a[0], math.Max(b[0], d[0]))
	y1 := math.Max(a[1], math.Max(b[1], d[1]))
	return Rect{int(x0) - 1, int(y0) - 1, int(x1-x0) + 3, int(y1-y0) + 3}
}

func (c *Canvas) Tri(a, b, d pt, fill Col) {
	in := triInside(a, b, d)
	c.paint(triBounds(a, b, d), func(x, y float64) float64 { return clampf(0.5+in(x, y), 0, 1) }, solid(fill))
}

func (c *Canvas) TriFn(a, b, d pt, col func(x, y float64) Col) {
	in := triInside(a, b, d)
	c.paint(triBounds(a, b, d), func(x, y float64) float64 { return clampf(0.5+in(x, y), 0, 1) }, col)
}

func (c *Canvas) TriStroke(a, b, d pt, width float64, col Col) {
	in := triInside(a, b, d)
	c.paint(triBounds(a, b, d), func(x, y float64) float64 {
		m := in(x, y)
		return clampf(0.5+m, 0, 1) - clampf(0.5+m-width, 0, 1)
	}, solid(col))
}

// Image draws img scaled into r, optionally with rounded corners.
func (c *Canvas) Image(img *image.RGBA, r Rect, radius float64) {
	if img == nil {
		return
	}
	sw, sh := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	fx, fy, fw, fh := float64(r.X), float64(r.Y), float64(r.W), float64(r.H)
	c.paint(r, func(x, y float64) float64 {
		if radius <= 0 {
			return 1
		}
		return cov01(sdfRound(x, y, fx, fy, fw, fh, radius))
	}, func(x, y float64) Col {
		u := (x-fx)/fw*sw - 0.5
		v := (y-fy)/fh*sh - 0.5
		pr, pg, pb, pa := sampleRGBA(img, u, v)
		if pa <= 0 {
			return Col{}
		}
		return Col{uint8(clampf(pr*255/pa, 0, 255)), uint8(clampf(pg*255/pa, 0, 255)), uint8(clampf(pb*255/pa, 0, 255)), uint8(clampf(pa, 0, 255))}
	})
}

// sampleRGBA does bilinear sampling on premultiplied pixels (returns 0..255 floats).
func sampleRGBA(img *image.RGBA, u, v float64) (r, g, b, a float64) {
	bd := img.Bounds()
	x0, y0 := int(math.Floor(u)), int(math.Floor(v))
	fx, fy := u-float64(x0), v-float64(y0)
	get := func(x, y int) (float64, float64, float64, float64) {
		x = maxi(bd.Min.X, mini(bd.Max.X-1, x+bd.Min.X))
		y = maxi(bd.Min.Y, mini(bd.Max.Y-1, y+bd.Min.Y))
		i := img.PixOffset(x, y)
		return float64(img.Pix[i]), float64(img.Pix[i+1]), float64(img.Pix[i+2]), float64(img.Pix[i+3])
	}
	r00, g00, b00, a00 := get(x0, y0)
	r10, g10, b10, a10 := get(x0+1, y0)
	r01, g01, b01, a01 := get(x0, y0+1)
	r11, g11, b11, a11 := get(x0+1, y0+1)
	mix := func(p00, p10, p01, p11 float64) float64 {
		top := p00 + (p10-p00)*fx
		bot := p01 + (p11-p01)*fx
		return top + (bot-top)*fy
	}
	return mix(r00, r10, r01, r11), mix(g00, g10, g01, g11), mix(b00, b10, b01, b11), mix(a00, a10, a01, a11)
}

// Line draws an anti-aliased line segment with round ends.
func (c *Canvas) Line(x0, y0, x1, y1, width float64, col Col) {
	pad := width/2 + 1
	b := Rect{int(math.Min(x0, x1) - pad), int(math.Min(y0, y1) - pad), int(math.Abs(x1-x0) + 2*pad + 1), int(math.Abs(y1-y0) + 2*pad + 1)}
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	c.paint(b, func(x, y float64) float64 {
		t := 0.0
		if l2 > 0 {
			t = clampf(((x-x0)*dx+(y-y0)*dy)/l2, 0, 1)
		}
		d := math.Hypot(x-(x0+t*dx), y-(y0+t*dy)) - width/2
		return cov01(d)
	}, solid(col))
}

// AreaUnder fills the region between a polyline and the line y=base, fading towards the base.
func (c *Canvas) AreaUnder(pts []pt, base float64, col Col, topAlpha float64) {
	if len(pts) < 2 {
		return
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		for x := int(math.Floor(a[0])); x <= int(math.Ceil(b[0])); x++ {
			fx := float64(x) + 0.5
			if fx < a[0] || fx > b[0] || b[0] == a[0] {
				continue
			}
			y := a[1] + (b[1]-a[1])*(fx-a[0])/(b[0]-a[0])
			if math.IsNaN(y) || math.IsInf(y, 0) {
				continue
			}
			y = math.Max(y, float64(c.clip.Y))
			for yy := int(y); yy < int(base) && yy < c.clip.Y+c.clip.H; yy++ {
				t := (float64(yy) - y) / math.Max(1, base-y)
				if yy < c.clip.Y || yy >= c.clip.Y+c.clip.H || x < c.clip.X || x >= c.clip.X+c.clip.W {
					continue
				}
				c.blend(x, yy, col, topAlpha*(1-t))
			}
		}
	}
}
