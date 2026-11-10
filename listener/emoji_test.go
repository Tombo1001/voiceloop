package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestAffine(t *testing.T) {
	m := affine{xx: 2, yy: 3, dx: 5, dy: -1}.then(affine{xx: 0, yx: 1, xy: -1, yy: 0}) // rotate 90°, then scale and move
	x, y := m.apply(1, 0)
	if !near(x, 5) || !near(y, 2) {
		t.Fatalf("apply: got %v,%v", x, y)
	}
	inv, ok := m.invert()
	if !ok {
		t.Fatal("not invertible")
	}
	bx, by := inv.apply(x, y)
	if !near(bx, 1) || !near(by, 0) {
		t.Fatalf("invert: got %v,%v", bx, by)
	}
}

func TestLinearT(t *testing.T) {
	// p0 (0,0) to p1 (100,0), bands vertical (p2 straight up): t is x/100.
	for _, c := range []struct{ x, want float64 }{{0, 0}, {50, .5}, {100, 1}, {150, 1.5}} {
		if got, _ := linearT(0, 0, 100, 0, 0, 100, c.x, 37); !near(got, c.want) {
			t.Errorf("x=%v: t=%v, want %v", c.x, got, c.want)
		}
	}
	// p2 tilted: bands follow p0-p2, so moving along p0-p2 keeps t the same.
	a, _ := linearT(0, 0, 100, 0, 50, 100, 10, 0)
	b, _ := linearT(0, 0, 100, 0, 50, 100, 10+25, 50)
	if !near(a, b) {
		t.Errorf("tilted bands: %v vs %v", a, b)
	}
}

func TestRadialT(t *testing.T) {
	// Concentric circles, r0=0 to r1=100: t is the distance from the centre / 100.
	for _, d := range []float64{0, 25, 100, 140} {
		got, ok := radialT(0, 0, 0, 0, 0, 100, d, 0)
		if !ok || !near(got, d/100) {
			t.Errorf("d=%v: t=%v ok=%v", d, got, ok)
		}
	}
}

func TestSampleStops(t *testing.T) {
	stops := []colorStop{{0, [4]float32{0, 0, 0, 1}}, {1, [4]float32{1, 1, 1, 1}}}
	if c := sampleStops(stops, 0, .5); math.Abs(float64(c[0])-.5) > 1e-6 {
		t.Errorf("middle: %v", c)
	}
	if c := sampleStops(stops, 0, 3); c[0] != 1 {
		t.Errorf("pad past the end: %v", c)
	}
	if c := sampleStops(stops, 1, 1.25); math.Abs(float64(c[0])-.25) > 1e-6 {
		t.Errorf("repeat: %v", c)
	}
	if c := sampleStops(stops, 2, 1.25); math.Abs(float64(c[0])-.75) > 1e-6 {
		t.Errorf("reflect: %v", c)
	}
}

// With VOICELOOP_EMOJI_FONT pointing at a copy of Segoe UI Emoji, check emoji render in
// colour and the pill fits them. EMOJI_PREVIEW=<file.png> also writes a preview sheet.
func TestEmojiInPill(t *testing.T) {
	if os.Getenv("VOICELOOP_EMOJI_FONT") == "" {
		t.Skip("set VOICELOOP_EMOJI_FONT to a COLR emoji font to run this")
	}
	e := emojiFace()
	if e == nil {
		t.Fatal("emoji font didn't load")
	}
	gid, ok := e.glyph('🧠')
	if !ok {
		t.Fatal("no colour glyph for 🧠")
	}
	img := e.image(gid, 40, color.NRGBA{255, 255, 255, 255})
	colours := map[[3]uint8]bool{}
	opaque := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] > 200 {
			opaque++
			colours[[3]uint8{img.Pix[i] / 32, img.Pix[i+1] / 32, img.Pix[i+2] / 32}] = true
		}
	}
	if opaque < 200 || len(colours) < 4 {
		t.Fatalf("🧠 looks empty or flat: %d opaque pixels, %d colour buckets", opaque, len(colours))
	}
	if rs := runs("Second Brain 🧠"); len(rs) != 2 || !rs[1].emoji || rs[0].s != "Second Brain " {
		t.Fatalf("runs: %+v", rs)
	}
	if rs := runs("ok ✅️ done"); len(rs) != 3 || !rs[1].emoji {
		t.Fatalf("variation selector: %+v", rs)
	}

	if out := os.Getenv("EMOJI_PREVIEW"); out != "" {
		titles := []string{"Second Brain 🧠", "Ship it 🚀🔥", "Tests ✅ and ❤️", "👍🏽 thumbs, 👨‍💻 coder", "plain text, no emoji"}
		var pills []*image.RGBA
		w, h := 0, 0
		for _, ti := range titles {
			p, _ := renderOverlay(1.5, defaultSettings().palette(), PillContent{Kind: "speech", Title: ti, Fit: true}, barLevels(nil, 0, nBars), 1)
			pills = append(pills, p)
			if p.Bounds().Dx() > w {
				w = p.Bounds().Dx()
			}
			h += p.Bounds().Dy() + 12
		}
		sheet := image.NewRGBA(image.Rect(0, 0, w+24, h+12))
		for i := range sheet.Pix {
			sheet.Pix[i] = 0x30
			if i%4 == 3 {
				sheet.Pix[i] = 0xff // an opaque dark sheet, like a desktop behind the pill
			}
		}
		y := 12
		for _, p := range pills {
			for py := 0; py < p.Bounds().Dy(); py++ {
				for px := 0; px < p.Bounds().Dx(); px++ {
					o, d := p.PixOffset(px, py), sheet.PixOffset(px+12, py+y)
					a := uint32(p.Pix[o+3])
					for c := 0; c < 3; c++ {
						sheet.Pix[d+c] = uint8(uint32(p.Pix[o+c]) + uint32(sheet.Pix[d+c])*(255-a)/255)
					}
				}
			}
			y += p.Bounds().Dy() + 12
		}
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, sheet); err != nil {
			t.Fatal(err)
		}
	}
}
