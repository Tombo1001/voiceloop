package main

// Overlay drawing. Pure Go, no Windows calls, so it can be previewed and tested on Linux.

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Base sizes at 96 dpi; multiplied by the display scale.
const (
	pillW = 236 // width with no thread name
	pillH = 52
	nBars = 9
)

var (
	colButton = color.NRGBA{0xff, 0xff, 0xff, 0x1c} // lightens whatever the pill colour is
	colCross  = color.NRGBA{0xd0, 0xd0, 0xd4, 0xff}
	colTitle  = color.NRGBA{0xf0, 0xf0, 0xf2, 0xff}
	colChip   = color.NRGBA{0xc8, 0xc8, 0xcc, 0xff}
)

// Accent colour per kind of audio.
func (p Palette) accent(kind string) color.NRGBA {
	switch kind {
	case "question":
		return p.Question
	case "approval":
		return p.Approval
	default:
		return p.Speech
	}
}

// PillContent is what the pill shows besides the bars.
type PillContent struct {
	Kind     string
	Title    string  // thread name; "" hides the name and the mute button
	Fit      bool    // grow to show the whole name
	MaxTitle float64 // longest name in 100%-scale px when not fitting
	Muted    bool    // the thread was just muted: the chip reads "Muted"
}

// PillLayout tells the window where the clickable parts are, in pixels.
type PillLayout struct {
	Stop, Mute image.Rectangle
}

// renderOverlay draws the pill. levels holds bar heights in 0..1; dotPulse in 0..1
// brightens the left dot. Returns a premultiplied RGBA image and the button areas.
func renderOverlay(scale float64, pal Palette, c PillContent, levels []float64, dotPulse float64) (*image.RGBA, PillLayout) {
	s := float32(scale)
	accent := pal.accent(c.Kind)
	barW, gap := 4.5*s, 13*s
	barsSpan := gap*float32(nBars-1) + barW

	var title, chip string
	var titleFace, chipFace font.Face
	var titleW, chipW float32
	if c.Title != "" {
		titleFace = face(14.5 * scale)
		chipFace = face(12 * scale)
		maxW := float32(c.MaxTitle) * s
		if c.Fit {
			maxW = 1400 * s // a sanity cap only
		}
		title = ellipsize(titleFace, c.Title, maxW)
		titleW = measure(titleFace, title)
		chip = "Mute thread"
		if c.Muted {
			chip = "Muted"
		}
		chipW = measure(chipFace, chip) + 22*s
	}

	// One row (dot, bars, ✕) without a name; two rows with one:
	// row 1 is dot, name, Mute thread and ✕; row 2 is the bars, centred in the pill.
	var w, h, row1, row2, maxBar float32
	if title == "" {
		w, h = float32(math.Round(pillW*scale)), float32(math.Round(pillH*scale))
		row1, row2, maxBar = h/2, h/2, h-18*s
	} else {
		w = 42*s + titleW + 12*s + chipW + 10*s + 28*s + 12*s
		w = float32(math.Max(float64(w), math.Round(pillW*scale)))
		h = float32(math.Round(86 * scale))
		row1, row2, maxBar = 27*s, 61*s, 28*s
	}
	wi, hi := int(math.Ceil(float64(w))), int(math.Ceil(float64(h)))
	img := image.NewRGBA(image.Rect(0, 0, wi, hi))
	fw, fh := float32(wi), float32(hi)

	if title == "" {
		capsule(img, 0, 0, fw, fh, pal.Background)
	} else {
		roundRect(img, 0, 0, fw, fh, 24*s, pal.Background)
	}

	dot := accent
	dot.A = uint8(150 + 105*clamp01(dotPulse))
	circle(img, 24*s, row1, 6*s, dot)

	// Bars, centred horizontally (beside the dot and ✕ in the one-row pill).
	barsX := fw/2 - barsSpan/2
	if title == "" {
		barsX = fw/2 - 8*s - barsSpan/2 // optically centred between the dot and the ✕
	}
	for i := 0; i < nBars; i++ {
		lv := 0.0
		if i < len(levels) {
			lv = clamp01(levels[i])
		}
		bh := barW + float32(lv)*(maxBar-barW)
		cx := barsX + gap*float32(i) + barW/2
		col := accent
		col.A = uint8(170 + 85*lv)
		capsule(img, cx-barW/2, row2-bh/2, barW, bh, col)
	}

	var lay PillLayout
	if title != "" {
		tx := 42 * s
		drawText(img, titleFace, title, tx, row1, colTitle)
		chipX := tx + titleW + 12*s
		chipH := 26 * s
		cc := colButton
		if c.Muted {
			cc = accent
			cc.A = 0x40
		}
		capsule(img, chipX, row1-chipH/2, chipW, chipH, cc)
		drawText(img, chipFace, chip, chipX+11*s, row1, colChip)
		lay.Mute = image.Rect(int(chipX-4*s), int(row1-chipH/2-6*s), int(chipX+chipW+4*s), int(row1+chipH/2+6*s))
	}

	br := 14 * s
	bx, by := fw-12*s-br, row1
	if title == "" {
		br = 15 * s
		bx = fw - 26*s
	}
	circle(img, bx, by, br, colButton)
	l := 5 * s
	line(img, bx-l, by-l, bx+l, by+l, 1.7*s, colCross)
	line(img, bx-l, by+l, bx+l, by-l, 1.7*s, colCross)
	lay.Stop = image.Rect(int(bx-br-5*s), int(by-br-5*s), wi, int(by+br+5*s))
	if title == "" {
		lay.Stop = image.Rect(int(bx-br-6*s), 0, wi, hi)
	}
	return img, lay
}

// roundRect fills a rectangle with corner radius r.
func roundRect(img *image.RGBA, x, y, w, h, r float32, c color.NRGBA) {
	k := r * kappa
	z := newZ(img)
	z.MoveTo(x+r, y)
	z.LineTo(x+w-r, y)
	z.CubeTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r)
	z.LineTo(x+w, y+h-r)
	z.CubeTo(x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h)
	z.LineTo(x+r, y+h)
	z.CubeTo(x+r-k, y+h, x, y+h-r+k, x, y+h-r)
	z.LineTo(x, y+r)
	z.CubeTo(x, y+r-k, x+r-k, y, x+r, y)
	z.ClosePath()
	fill(img, z, c)
}

// ---------- text ----------

var (
	fontOnce  sync.Once
	pillFont  *opentype.Font
	faceMu    sync.Mutex
	faceCache = map[float64]font.Face{}
)

// Segoe UI Semibold on Windows; Go's own font elsewhere (previews on Linux).
func loadFont() {
	for _, p := range []string{`C:\Windows\Fonts\seguisb.ttf`, `C:\Windows\Fonts\segoeui.ttf`} {
		if b, err := os.ReadFile(p); err == nil {
			if f, err := opentype.Parse(b); err == nil {
				pillFont = f
				return
			}
		}
	}
	pillFont, _ = opentype.Parse(gomedium.TTF)
}

func face(px float64) font.Face {
	fontOnce.Do(loadFont)
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faceCache[px]; ok {
		return f
	}
	f, err := opentype.NewFace(pillFont, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil
	}
	faceCache[px] = f
	return f
}

func measure(f font.Face, s string) float32 {
	return float32(font.MeasureString(f, s)) / 64
}

// ellipsize shortens s with "…" until it fits in max pixels.
func ellipsize(f font.Face, s string, max float32) string {
	if measure(f, s) <= max {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measure(f, strings.TrimSpace(string(r[:mid]))+"…") <= max {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimSpace(string(r[:lo])) + "…"
}

// drawText draws s vertically centred on cy.
func drawText(img *image.RGBA, f font.Face, s string, x, cy float32, c color.NRGBA) {
	m := f.Metrics()
	asc, desc := float32(m.Ascent)/64, float32(m.Descent)/64
	baseline := cy + (asc-desc)/2
	d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: f,
		Dot: fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(baseline * 64)}}
	d.DrawString(s)
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

const kappa = 0.5523 // cubic Bézier circle constant

func fill(img *image.RGBA, z *vector.Rasterizer, c color.NRGBA) {
	z.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
}

func newZ(img *image.RGBA) *vector.Rasterizer {
	b := img.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	z.DrawOp = draw.Over
	return z
}

func circle(img *image.RGBA, cx, cy, r float32, c color.NRGBA) {
	z := newZ(img)
	k := r * kappa
	z.MoveTo(cx+r, cy)
	z.CubeTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	z.CubeTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	z.CubeTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	z.CubeTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	z.ClosePath()
	fill(img, z, c)
}

// capsule fills a rectangle with fully rounded ends (radius = half the shorter side).
func capsule(img *image.RGBA, x, y, w, h float32, c color.NRGBA) {
	r := w / 2
	if h < w {
		r = h / 2
	}
	k := r * kappa
	z := newZ(img)
	z.MoveTo(x+r, y)
	z.LineTo(x+w-r, y)
	z.CubeTo(x+w-r+k, y, x+w, y+r-k, x+w, y+r)
	z.LineTo(x+w, y+h-r)
	z.CubeTo(x+w, y+h-r+k, x+w-r+k, y+h, x+w-r, y+h)
	z.LineTo(x+r, y+h)
	z.CubeTo(x+r-k, y+h, x, y+h-r+k, x, y+h-r)
	z.LineTo(x, y+r)
	z.CubeTo(x, y+r-k, x+r-k, y, x+r, y)
	z.ClosePath()
	fill(img, z, c)
}

func line(img *image.RGBA, x1, y1, x2, y2, width float32, c color.NRGBA) {
	dx, dy := x2-x1, y2-y1
	n := float32(math.Hypot(float64(dx), float64(dy)))
	if n == 0 {
		return
	}
	px, py := -dy/n*width/2, dx/n*width/2
	z := newZ(img)
	z.MoveTo(x1+px, y1+py)
	z.LineTo(x2+px, y2+py)
	z.LineTo(x2-px, y2-py)
	z.LineTo(x1-px, y1-py)
	z.ClosePath()
	fill(img, z, c)
}

// ---------- animation ----------

const envStepMs = 40

// envelope turns 16-bit mono PCM into loudness steps of envStepMs, normalised to 0..1.
func envelope(pcm []byte, sampleRate int) []float64 {
	step := sampleRate * envStepMs / 1000
	if step <= 0 {
		return nil
	}
	n := len(pcm) / 2
	var env []float64
	peak := 1e-9
	for i := 0; i < n; i += step {
		var sum float64
		end := i + step
		if end > n {
			end = n
		}
		for j := i; j < end; j++ {
			v := float64(int16(uint16(pcm[2*j]) | uint16(pcm[2*j+1])<<8))
			sum += v * v
		}
		rms := math.Sqrt(sum / float64(end-i))
		env = append(env, rms)
		peak = math.Max(peak, rms)
	}
	for i := range env {
		env[i] = math.Pow(env[i]/peak, 0.6) // lift quiet syllables
	}
	return env
}

// barLevels maps playback position to bar heights: the centre follows the audio and
// the outer bars echo it slightly later, so the row ripples outwards.
func barLevels(env []float64, elapsedMs, n int) []float64 {
	out := make([]float64, n)
	mid := n / 2
	for i := range out {
		d := i - mid
		if d < 0 {
			d = -d
		}
		idx := elapsedMs/envStepMs - d
		if idx >= 0 && idx < len(env) {
			out[i] = env[idx] * (1 - 0.13*float64(d))
		}
	}
	return out
}

// pendingLevels is the "getting ready to speak" animation: a soft wave along the dots.
func pendingLevels(elapsedMs, n int) []float64 {
	out := make([]float64, n)
	t := float64(elapsedMs) / 1000
	for i := range out {
		out[i] = 0.18 * (0.5 + 0.5*math.Sin(t*6-float64(i)*0.7))
	}
	return out
}
