package main

// Colour emoji for the pill's text. Segoe UI has no emoji, so a thread called
// "Second Brain 🧠" showed an empty box. This draws emoji from Segoe UI Emoji
// (C:\Windows\Fonts\seguiemj.ttf), which keeps its colour glyphs in the COLR table:
// version 0 (flat colour layers, Windows 10) or version 1 (layers with linear and
// radial gradients, transforms and compositing, Windows 11). Pure Go and no Windows
// calls, so it runs in the Linux tests when VOICELOOP_EMOJI_FONT names the font file.
//
// Not handled: ZWJ sequences and skin-tone modifiers need the font's GSUB ligatures,
// so "👨‍💻" draws as 👨 then 💻, and a skin tone falls back to the default emoji.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

type emojiFont struct {
	sf        *sfnt.Font
	upem      float64
	asc, desc float64 // the font bounding box above and below the baseline (both positive), font units
	left      float64 // how far the bounding box reaches left of the origin (0 or positive)
	palette   []color.NRGBA
	colr      []byte
	v0Base    map[sfnt.GlyphIndex][2]int // first layer, layer count
	v0Layers  [][2]uint16                // glyph, palette index
	v1Base    map[sfnt.GlyphIndex]int    // paint offset within colr
	layerList []int                      // paint offsets within colr

	mu    sync.Mutex
	buf   sfnt.Buffer
	cache map[emojiKey]*image.RGBA
}

type emojiKey struct {
	gid sfnt.GlyphIndex
	px  int // size in 1/100 px
	fg  color.NRGBA
}

func loadEmojiFont(path string) (*emojiFont, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sf, err := sfnt.Parse(b)
	if err != nil {
		return nil, err
	}
	tables, err := tableDirectory(b)
	if err != nil {
		return nil, err
	}
	e := &emojiFont{sf: sf, cache: map[emojiKey]*image.RGBA{}}
	head, hhea := tables["head"], tables["hhea"]
	if len(head) < 44 || len(hhea) < 8 {
		return nil, errors.New("emoji font: missing head or hhea")
	}
	// The emoji rise above the font's ascender, so the canvas spans the whole bounding box.
	i16 := func(b []byte, p int) float64 { return float64(int16(binary.BigEndian.Uint16(b[p:]))) }
	e.upem = float64(binary.BigEndian.Uint16(head[18:]))
	e.left = math.Max(0, -i16(head, 36))
	e.asc = math.Max(i16(head, 42), i16(hhea, 4))
	e.desc = math.Max(-i16(head, 38), -i16(hhea, 6))
	if err := e.parseCPAL(tables["CPAL"]); err != nil {
		return nil, err
	}
	if err := e.parseCOLR(tables["COLR"]); err != nil {
		return nil, err
	}
	return e, nil
}

// tableDirectory maps each table tag to its bytes.
func tableDirectory(b []byte) (map[string][]byte, error) {
	if len(b) < 12 {
		return nil, errors.New("font too short")
	}
	n := int(binary.BigEndian.Uint16(b[4:]))
	out := map[string][]byte{}
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		if rec+16 > len(b) {
			return nil, errors.New("truncated table directory")
		}
		off, ln := int(binary.BigEndian.Uint32(b[rec+8:])), int(binary.BigEndian.Uint32(b[rec+12:]))
		if off < 0 || off+ln > len(b) {
			return nil, fmt.Errorf("table %q out of range", b[rec:rec+4])
		}
		out[string(b[rec:rec+4])] = b[off : off+ln]
	}
	return out, nil
}

func (e *emojiFont) parseCPAL(t []byte) error {
	if len(t) < 12 {
		return errors.New("emoji font: no CPAL table")
	}
	numEntries := int(binary.BigEndian.Uint16(t[2:]))
	recordsOff := int(binary.BigEndian.Uint32(t[8:]))
	first := int(binary.BigEndian.Uint16(t[12:]))
	for i := 0; i < numEntries; i++ {
		p := recordsOff + 4*(first+i)
		if p+4 > len(t) {
			return errors.New("emoji font: CPAL out of range")
		}
		e.palette = append(e.palette, color.NRGBA{B: t[p], G: t[p+1], R: t[p+2], A: t[p+3]})
	}
	return nil
}

func (e *emojiFont) parseCOLR(t []byte) error {
	if len(t) < 14 {
		return errors.New("emoji font: no COLR table")
	}
	e.colr = t
	version := binary.BigEndian.Uint16(t)
	nBase := int(binary.BigEndian.Uint16(t[2:]))
	baseOff, layerOff := int(binary.BigEndian.Uint32(t[4:])), int(binary.BigEndian.Uint32(t[8:]))
	nLayers := int(binary.BigEndian.Uint16(t[12:]))
	e.v0Base = map[sfnt.GlyphIndex][2]int{}
	for i := 0; i < nBase && baseOff+6*i+6 <= len(t); i++ {
		p := baseOff + 6*i
		e.v0Base[sfnt.GlyphIndex(binary.BigEndian.Uint16(t[p:]))] = [2]int{int(binary.BigEndian.Uint16(t[p+2:])), int(binary.BigEndian.Uint16(t[p+4:]))}
	}
	for i := 0; i < nLayers && layerOff+4*i+4 <= len(t); i++ {
		p := layerOff + 4*i
		e.v0Layers = append(e.v0Layers, [2]uint16{binary.BigEndian.Uint16(t[p:]), binary.BigEndian.Uint16(t[p+2:])})
	}
	e.v1Base = map[sfnt.GlyphIndex]int{}
	if version < 1 || len(t) < 22 {
		return nil
	}
	if bgl := int(binary.BigEndian.Uint32(t[14:])); bgl != 0 && bgl+4 <= len(t) {
		n := int(binary.BigEndian.Uint32(t[bgl:]))
		for i := 0; i < n && bgl+4+6*i+6 <= len(t); i++ {
			p := bgl + 4 + 6*i
			e.v1Base[sfnt.GlyphIndex(binary.BigEndian.Uint16(t[p:]))] = bgl + int(binary.BigEndian.Uint32(t[p+2:]))
		}
	}
	if ll := int(binary.BigEndian.Uint32(t[18:])); ll != 0 && ll+4 <= len(t) {
		n := int(binary.BigEndian.Uint32(t[ll:]))
		for i := 0; i < n && ll+4+4*i+4 <= len(t); i++ {
			e.layerList = append(e.layerList, ll+int(binary.BigEndian.Uint32(t[ll+4+4*i:])))
		}
	}
	return nil
}

// glyph returns the font's glyph for r when the font has a colour version of it.
func (e *emojiFont) glyph(r rune) (sfnt.GlyphIndex, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	gid, err := e.sf.GlyphIndex(&e.buf, r)
	if err != nil || gid == 0 {
		return 0, false
	}
	_, v1 := e.v1Base[gid]
	_, v0 := e.v0Base[gid]
	return gid, v1 || v0
}

// advance is the glyph's width in pixels at size px.
func (e *emojiFont) advance(gid sfnt.GlyphIndex, px float64) float32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, err := e.sf.GlyphAdvance(&e.buf, gid, fixed.I(int(e.upem)), font.HintingNone)
	if err != nil {
		return float32(px)
	}
	return float32(float64(a) / 64 * px / e.upem)
}

// image renders the glyph at size px. Its top-left corner sits asc*px/upem above the
// baseline and left*px/upem left of the pen. fg is the text colour, for palette index 0xFFFF.
func (e *emojiFont) image(gid sfnt.GlyphIndex, px float64, fg color.NRGBA) *image.RGBA {
	key := emojiKey{gid, int(px * 100), fg}
	e.mu.Lock()
	defer e.mu.Unlock()
	if img, ok := e.cache[key]; ok {
		return img
	}
	s := px / e.upem
	adv, err := e.sf.GlyphAdvance(&e.buf, gid, fixed.I(int(e.upem)), font.HintingNone)
	if err != nil {
		adv = fixed.I(int(e.upem))
	}
	r := &colrRenderer{e: e, fg: fg,
		w:   int(math.Ceil((float64(adv)/64 + 2*e.left) * s)),
		h:   int(math.Ceil((e.asc + e.desc) * s)),
		dev: affine{xx: s, yy: -s, dx: e.left * s, dy: e.asc * s}}
	var out []float32
	if off, ok := e.v1Base[gid]; ok {
		out = r.paint(off, identity, 0)
	} else if v0, ok := e.v0Base[gid]; ok {
		out = r.layersV0(v0[0], v0[1])
	}
	img := r.toRGBA(out)
	e.cache[key] = img
	return img
}

// ---------- rendering ----------

type affine struct{ xx, yx, xy, yy, dx, dy float64 } // x' = xx*x + xy*y + dx; y' = yx*x + yy*y + dy

var identity = affine{xx: 1, yy: 1}

// then returns a∘b: apply b first, then a.
func (a affine) then(b affine) affine {
	return affine{
		xx: a.xx*b.xx + a.xy*b.yx, yx: a.yx*b.xx + a.yy*b.yx,
		xy: a.xx*b.xy + a.xy*b.yy, yy: a.yx*b.xy + a.yy*b.yy,
		dx: a.xx*b.dx + a.xy*b.dy + a.dx, dy: a.yx*b.dx + a.yy*b.dy + a.dy,
	}
}

func (a affine) apply(x, y float64) (float64, float64) {
	return a.xx*x + a.xy*y + a.dx, a.yx*x + a.yy*y + a.dy
}

func (a affine) invert() (affine, bool) {
	det := a.xx*a.yy - a.xy*a.yx
	if math.Abs(det) < 1e-12 {
		return affine{}, false
	}
	i := affine{xx: a.yy / det, yx: -a.yx / det, xy: -a.xy / det, yy: a.xx / det}
	i.dx, i.dy = -(i.xx*a.dx + i.xy*a.dy), -(i.yx*a.dx + i.yy*a.dy)
	return i, true
}

type colrRenderer struct {
	e    *emojiFont
	fg   color.NRGBA
	w, h int
	dev  affine // font units (y up) to canvas pixels (y down)
}

// paint renders one COLRv1 paint table to a premultiplied RGBA canvas (4 floats a pixel).
// ctm maps the paint's own space to font space.
func (r *colrRenderer) paint(off int, ctm affine, depth int) []float32 {
	t := r.e.colr
	out := make([]float32, r.w*r.h*4)
	if depth > 64 || off <= 0 || off >= len(t) {
		return out
	}
	u8 := func(p int) int { return int(t[off+p]) }
	u16 := func(p int) int { return int(binary.BigEndian.Uint16(t[off+p:])) }
	i16 := func(p int) float64 { return float64(int16(binary.BigEndian.Uint16(t[off+p:]))) }
	f2 := func(p int) float64 { return i16(p) / 16384 }
	u24 := func(p int) int { return int(t[off+p])<<16 | int(t[off+p+1])<<8 | int(t[off+p+2]) }
	child := func(p int) int { return off + u24(p) }
	around := func(m affine, cx, cy float64) affine {
		return affine{xx: 1, yy: 1, dx: cx, dy: cy}.then(m).then(affine{xx: 1, yy: 1, dx: -cx, dy: -cy})
	}
	switch format := u8(0); format {
	case 1: // PaintColrLayers
		n, first := u8(1), int(binary.BigEndian.Uint32(t[off+2:]))
		for i := first; i < first+n && i < len(r.e.layerList); i++ {
			srcOver(out, r.paint(r.e.layerList[i], ctm, depth+1))
		}
	case 2, 3: // PaintSolid
		c := r.colour(u16(1), f2(3))
		for i := 0; i < len(out); i += 4 {
			copy(out[i:i+4], c[:])
		}
	case 4, 5: // PaintLinearGradient
		cl := child(1)
		x0, y0, x1, y1, x2, y2 := i16(4), i16(6), i16(8), i16(10), i16(12), i16(14)
		r.gradient(out, ctm, cl, func(x, y float64) (float64, bool) { return linearT(x0, y0, x1, y1, x2, y2, x, y) })
	case 6, 7: // PaintRadialGradient
		cl := child(1)
		x0, y0, r0, x1, y1, r1 := i16(4), i16(6), float64(u16(8)), i16(10), i16(12), float64(u16(14))
		r.gradient(out, ctm, cl, func(x, y float64) (float64, bool) { return radialT(x0, y0, r0, x1, y1, r1, x, y) })
	case 8, 9: // PaintSweepGradient
		cl := child(1)
		cx, cy, start, end := i16(4), i16(6), f2(8)*180+180, f2(10)*180+180
		r.gradient(out, ctm, cl, func(x, y float64) (float64, bool) {
			a := math.Atan2(y-cy, x-cx) * 180 / math.Pi
			if a < 0 {
				a += 360
			}
			if end == start {
				return 0, true
			}
			return (a - start) / (end - start), true
		})
	case 10: // PaintGlyph: the child paint, clipped to the glyph's outline
		fill := r.paint(child(1), ctm, depth+1)
		mask := r.glyphMask(sfnt.GlyphIndex(u16(4)), ctm)
		for i, a := range mask.Pix {
			if a == 0 {
				continue
			}
			k := float32(a) / 255
			for c := 0; c < 4; c++ {
				out[i*4+c] = fill[i*4+c] * k
			}
		}
	case 11: // PaintColrGlyph
		gid := sfnt.GlyphIndex(u16(1))
		if p, ok := r.e.v1Base[gid]; ok {
			return r.paint(p, ctm, depth+1)
		}
		if v0, ok := r.e.v0Base[gid]; ok {
			return r.layersV0(v0[0], v0[1])
		}
	case 12, 13: // PaintTransform
		m := t[off+u24(4):]
		fx := func(i int) float64 { return float64(int32(binary.BigEndian.Uint32(m[4*i:]))) / 65536 }
		return r.paint(child(1), ctm.then(affine{xx: fx(0), yx: fx(1), xy: fx(2), yy: fx(3), dx: fx(4), dy: fx(5)}), depth+1)
	case 14, 15: // PaintTranslate
		return r.paint(child(1), ctm.then(affine{xx: 1, yy: 1, dx: i16(4), dy: i16(6)}), depth+1)
	case 16, 17: // PaintScale
		return r.paint(child(1), ctm.then(affine{xx: f2(4), yy: f2(6)}), depth+1)
	case 18, 19: // PaintScaleAroundCenter
		return r.paint(child(1), ctm.then(around(affine{xx: f2(4), yy: f2(6)}, i16(8), i16(10))), depth+1)
	case 20, 21: // PaintScaleUniform
		return r.paint(child(1), ctm.then(affine{xx: f2(4), yy: f2(4)}), depth+1)
	case 22, 23: // PaintScaleUniformAroundCenter
		return r.paint(child(1), ctm.then(around(affine{xx: f2(4), yy: f2(4)}, i16(6), i16(8))), depth+1)
	case 24, 25, 26, 27: // PaintRotate (and around a centre)
		a := f2(4) * math.Pi
		m := affine{xx: math.Cos(a), yx: math.Sin(a), xy: -math.Sin(a), yy: math.Cos(a)}
		if format >= 26 {
			m = around(m, i16(6), i16(8))
		}
		return r.paint(child(1), ctm.then(m), depth+1)
	case 28, 29, 30, 31: // PaintSkew (and around a centre)
		m := affine{xx: 1, yy: 1, xy: -math.Tan(f2(4) * math.Pi), yx: math.Tan(f2(6) * math.Pi)}
		if format >= 30 {
			m = around(m, i16(8), i16(10))
		}
		return r.paint(child(1), ctm.then(m), depth+1)
	case 32: // PaintComposite
		src := r.paint(child(1), ctm, depth+1)
		dst := r.paint(off+u24(5), ctm, depth+1)
		composite(dst, src, u8(4))
		return dst
	}
	return out
}

// layersV0 renders a COLRv0 glyph: flat colour layers, painted in order.
func (r *colrRenderer) layersV0(first, n int) []float32 {
	out := make([]float32, r.w*r.h*4)
	for i := first; i < first+n && i < len(r.e.v0Layers); i++ {
		l := r.e.v0Layers[i]
		c := r.colour(int(l[1]), 1)
		mask := r.glyphMask(sfnt.GlyphIndex(l[0]), identity)
		layer := make([]float32, len(out))
		for j, a := range mask.Pix {
			k := float32(a) / 255
			for ch := 0; ch < 4; ch++ {
				layer[j*4+ch] = c[ch] * k
			}
		}
		srcOver(out, layer)
	}
	return out
}

// colour is a palette entry, premultiplied, with extra alpha applied. 0xFFFF is the text colour.
func (r *colrRenderer) colour(idx int, alpha float64) [4]float32 {
	c := r.fg
	if idx != 0xFFFF && idx < len(r.e.palette) {
		c = r.e.palette[idx]
	}
	a := float32(c.A) / 255 * float32(math.Max(0, math.Min(1, alpha)))
	return [4]float32{float32(c.R) / 255 * a, float32(c.G) / 255 * a, float32(c.B) / 255 * a, a}
}

// gradient fills out by mapping each pixel centre back into the paint's own space.
func (r *colrRenderer) gradient(out []float32, ctm affine, cl int, tAt func(x, y float64) (float64, bool)) {
	inv, ok := r.dev.then(ctm).invert()
	if !ok {
		return
	}
	stops, extend := r.colorLine(cl)
	if len(stops) == 0 {
		return
	}
	for py := 0; py < r.h; py++ {
		for px := 0; px < r.w; px++ {
			x, y := inv.apply(float64(px)+0.5, float64(py)+0.5)
			tv, ok := tAt(x, y)
			if !ok {
				continue
			}
			c := sampleStops(stops, extend, tv)
			copy(out[(py*r.w+px)*4:], c[:])
		}
	}
}

type colorStop struct {
	t float64
	c [4]float32
}

func (r *colrRenderer) colorLine(off int) ([]colorStop, int) {
	t := r.e.colr
	if off+3 > len(t) {
		return nil, 0
	}
	extend, n := int(t[off]), int(binary.BigEndian.Uint16(t[off+1:]))
	var stops []colorStop
	for i := 0; i < n && off+3+6*i+6 <= len(t); i++ {
		p := off + 3 + 6*i
		st := float64(int16(binary.BigEndian.Uint16(t[p:]))) / 16384
		idx := int(binary.BigEndian.Uint16(t[p+2:]))
		alpha := float64(int16(binary.BigEndian.Uint16(t[p+4:]))) / 16384
		stops = append(stops, colorStop{st, r.colour(idx, alpha)})
	}
	for i := 1; i < len(stops); i++ { // the spec allows unsorted stops
		for j := i; j > 0 && stops[j].t < stops[j-1].t; j-- {
			stops[j], stops[j-1] = stops[j-1], stops[j]
		}
	}
	return stops, extend
}

// sampleStops interpolates the colour line at t, after the extend mode (0 pad, 1 repeat, 2 reflect).
func sampleStops(stops []colorStop, extend int, t float64) [4]float32 {
	first, last := stops[0].t, stops[len(stops)-1].t
	if span := last - first; span > 0 {
		switch extend {
		case 1:
			t = first + math.Mod(math.Mod(t-first, span)+span, span)
		case 2:
			u := math.Mod(math.Mod(t-first, 2*span)+2*span, 2*span)
			if u > span {
				u = 2*span - u
			}
			t = first + u
		}
	}
	if t <= first {
		return stops[0].c
	}
	if t >= last {
		return stops[len(stops)-1].c
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].t {
			a, b := stops[i-1], stops[i]
			k := float32(0)
			if b.t > a.t {
				k = float32((t - a.t) / (b.t - a.t))
			}
			var c [4]float32
			for ch := range c {
				c[ch] = a.c[ch] + (b.c[ch]-a.c[ch])*k
			}
			return c
		}
	}
	return stops[len(stops)-1].c
}

// linearT is the COLRv1 linear gradient: p0 to p1, with p2 setting the angle of the
// colour bands (they run parallel to p0-p2).
func linearT(x0, y0, x1, y1, x2, y2, x, y float64) (float64, bool) {
	px, py := -(y2 - y0), x2-x0 // perpendicular to p0-p2
	pp := px*px + py*py
	if pp == 0 {
		px, py, pp = x1-x0, y1-y0, (x1-x0)*(x1-x0)+(y1-y0)*(y1-y0)
		if pp == 0 {
			return 0, true
		}
	}
	k := ((x1-x0)*px + (y1-y0)*py) / pp
	vx, vy := px*k, py*k // p0 to the projection of p1
	vv := vx*vx + vy*vy
	if vv == 0 {
		return 0, true
	}
	return ((x-x0)*vx + (y-y0)*vy) / vv, true
}

// radialT is the two-circle (conical) gradient: the largest t whose circle passes through
// (x, y) with a radius that isn't negative.
func radialT(x0, y0, r0, x1, y1, r1, x, y float64) (float64, bool) {
	cdx, cdy, dr := x1-x0, y1-y0, r1-r0
	pdx, pdy := x-x0, y-y0
	a := cdx*cdx + cdy*cdy - dr*dr
	b := pdx*cdx + pdy*cdy + r0*dr
	c := pdx*pdx + pdy*pdy - r0*r0
	if math.Abs(a) < 1e-9 {
		if b == 0 {
			return 0, false
		}
		t := c / (2 * b)
		return t, r0+t*dr >= 0
	}
	disc := b*b - a*c
	if disc < 0 {
		return 0, false
	}
	sq := math.Sqrt(disc)
	t1, t2 := (b+sq)/a, (b-sq)/a
	if t1 < t2 {
		t1, t2 = t2, t1
	}
	if r0+t1*dr >= 0 {
		return t1, true
	}
	if r0+t2*dr >= 0 {
		return t2, true
	}
	return 0, false
}

// glyphMask rasterises a glyph outline, transformed by ctm, into canvas coverage.
func (r *colrRenderer) glyphMask(gid sfnt.GlyphIndex, ctm affine) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, r.w, r.h))
	segs, err := r.e.sf.LoadGlyph(&r.e.buf, gid, fixed.I(int(r.e.upem)), nil)
	if err != nil || len(segs) == 0 {
		return mask
	}
	m := r.dev.then(ctm)
	pt := func(p fixed.Point26_6) (float32, float32) {
		x, y := m.apply(float64(p.X)/64, -float64(p.Y)/64) // sfnt's y points down; font space points up
		return float32(x), float32(y)
	}
	z := vector.NewRasterizer(r.w, r.h)
	z.DrawOp = draw.Src
	open := false
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if open {
				z.ClosePath()
			}
			z.MoveTo(pt(s.Args[0]))
			open = true
		case sfnt.SegmentOpLineTo:
			z.LineTo(pt(s.Args[0]))
		case sfnt.SegmentOpQuadTo:
			ax, ay := pt(s.Args[0])
			bx, by := pt(s.Args[1])
			z.QuadTo(ax, ay, bx, by)
		case sfnt.SegmentOpCubeTo:
			ax, ay := pt(s.Args[0])
			bx, by := pt(s.Args[1])
			cx, cy := pt(s.Args[2])
			z.CubeTo(ax, ay, bx, by, cx, cy)
		}
	}
	if open {
		z.ClosePath()
	}
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	return mask
}

// srcOver composites src over dst, both premultiplied.
func srcOver(dst, src []float32) {
	for i := 0; i < len(dst); i += 4 {
		k := 1 - src[i+3]
		for c := 0; c < 4; c++ {
			dst[i+c] = src[i+c] + dst[i+c]*k
		}
	}
}

// composite applies a COLRv1 composite mode with src over the backdrop dst, in place.
// The Porter-Duff modes, plus screen and multiply; other blend modes fall back to src-over.
func composite(dst, src []float32, mode int) {
	for i := 0; i < len(dst); i += 4 {
		sa, da := src[i+3], dst[i+3]
		var fs, fd float32 // coverage factors for src and dst
		switch mode {
		case 0: // clear
		case 1: // src
			fs = 1
		case 2: // dest
			fd = 1
		case 4: // dest over
			fs, fd = 1-da, 1
		case 5: // src in
			fs = da
		case 6: // dest in
			fd = sa
		case 7: // src out
			fs = 1 - da
		case 8: // dest out
			fd = 1 - sa
		case 9: // src atop
			fs, fd = da, 1-sa
		case 10: // dest atop
			fs, fd = 1-da, sa
		case 11: // xor
			fs, fd = 1-da, 1-sa
		case 12: // plus
			fs, fd = 1, 1
		case 13: // screen
			for c := 0; c < 4; c++ {
				dst[i+c] = src[i+c] + dst[i+c] - src[i+c]*dst[i+c]
			}
			continue
		case 23: // multiply
			for c := 0; c < 3; c++ {
				dst[i+c] = src[i+c]*dst[i+c] + src[i+c]*(1-da) + dst[i+c]*(1-sa)
			}
			dst[i+3] = sa + da - sa*da
			continue
		default: // src over
			fs, fd = 1, 1-sa
		}
		for c := 0; c < 4; c++ {
			v := src[i+c]*fs + dst[i+c]*fd
			if v > 1 {
				v = 1
			}
			dst[i+c] = v
		}
	}
}

func (r *colrRenderer) toRGBA(px []float32) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.w, r.h))
	for i := range px {
		v := px[i]
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		img.Pix[i] = uint8(v*255 + 0.5)
	}
	return img
}
