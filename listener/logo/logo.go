// Package logo draws the voiceloop "vl" mark: a teal v and a white l on a dark circle.
package logo

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"

	"golang.org/x/image/vector"
)

var (
	Teal  = color.NRGBA{0x4f, 0xd1, 0xc5, 0xff}
	White = color.NRGBA{0xf2, 0xf2, 0xf4, 0xff}
	Grey  = color.NRGBA{0x80, 0x80, 0x86, 0xff}
	Dark  = color.NRGBA{0x1d, 0x1d, 0x1f, 0xff}
)

// Render draws the mark at size×size pixels. vCol is the v's colour (Teal, or Grey when off).
func Render(size int, vCol color.NRGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float32(size)
	circle(img, s/2, s/2, s/2-0.5, Dark)
	w := s * 0.115 // stroke width
	if size <= 20 {
		w = s * 0.14 // thicker at tray sizes so it stays legible
	}
	// v: two strokes meeting at the bottom, l: one tall stroke. Round caps and joins.
	vx0, vx1, vx2 := s*0.24, s*0.395, s*0.55
	vy0, vy1 := s*0.36, s*0.68
	stroke(img, vx0, vy0, vx1, vy1, w, vCol)
	stroke(img, vx1, vy1, vx2, vy0, w, vCol)
	stroke(img, s*0.715, s*0.27, s*0.715, vy1, w, White)
	return img
}

// ICO packs PNG renders at the given sizes into one .ico file.
func ICO(vCol color.NRGBA, sizes ...int) []byte {
	var pngs [][]byte
	for _, n := range sizes {
		var b bytes.Buffer
		_ = png.Encode(&b, Render(n, vCol))
		pngs = append(pngs, b.Bytes())
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, n := range sizes {
		d := byte(n)
		if n >= 256 {
			d = 0
		}
		out.Write([]byte{d, d, 0, 0})
		binary.Write(&out, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&out, binary.LittleEndian, []uint32{uint32(len(pngs[i])), uint32(offset)})
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return out.Bytes()
}

// PNG returns one render as PNG bytes.
func PNG(size int, vCol color.NRGBA) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, Render(size, vCol))
	return b.Bytes()
}

func newZ(img *image.RGBA) *vector.Rasterizer {
	z := vector.NewRasterizer(img.Bounds().Dx(), img.Bounds().Dy())
	z.DrawOp = draw.Over
	return z
}

func circle(img *image.RGBA, cx, cy, r float32, c color.NRGBA) {
	const k = 0.5523
	z := newZ(img)
	z.MoveTo(cx+r, cy)
	z.CubeTo(cx+r, cy+r*k, cx+r*k, cy+r, cx, cy+r)
	z.CubeTo(cx-r*k, cy+r, cx-r, cy+r*k, cx-r, cy)
	z.CubeTo(cx-r, cy-r*k, cx-r*k, cy-r, cx, cy-r)
	z.CubeTo(cx+r*k, cy-r, cx+r, cy-r*k, cx+r, cy)
	z.ClosePath()
	z.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
}

// stroke draws a line with round caps.
func stroke(img *image.RGBA, x1, y1, x2, y2, w float32, c color.NRGBA) {
	dx, dy := x2-x1, y2-y1
	n := float32(math.Hypot(float64(dx), float64(dy)))
	px, py := -dy/n*w/2, dx/n*w/2
	z := newZ(img)
	z.MoveTo(x1+px, y1+py)
	z.LineTo(x2+px, y2+py)
	z.LineTo(x2-px, y2-py)
	z.LineTo(x1-px, y1-py)
	z.ClosePath()
	z.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
	circle(img, x1, y1, w/2, c)
	circle(img, x2, y2, w/2, c)
}
