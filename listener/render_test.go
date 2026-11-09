package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

// Writes a contact sheet of overlay states to $PREVIEW (skipped when unset).
func TestRenderPreview(t *testing.T) {
	out := os.Getenv("PREVIEW")
	if out == "" {
		t.Skip("set PREVIEW")
	}
	raw, err := os.ReadFile(os.Getenv("KOKORO_WAV"))
	if err != nil {
		t.Skip("set KOKORO_WAV")
	}
	fixed, _, err := fixWav(raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	env := envelope(fixed[44:], 24000)
	scale := 1.5
	pal := defaultSettings().palette()
	long := "Move the photo library to the new NAS and check the backups still run"
	mk := func(c PillContent, lv []float64) *image.RGBA {
		img, _ := renderOverlay(scale, pal, c, lv, 1)
		return img
	}
	frames := []*image.RGBA{
		mk(PillContent{Kind: "speech"}, barLevels(env, 1200, nBars)),
		mk(PillContent{Kind: "speech", Title: "VoiceLoop", MaxTitle: 260}, barLevels(env, 1200, nBars)),
		mk(PillContent{Kind: "speech", Title: long, MaxTitle: 260}, barLevels(env, 2600, nBars)),
		mk(PillContent{Kind: "question", Title: long, Fit: true}, barLevels(envelope(chimePCM("question"), toneRate), 300, nBars)),
		mk(PillContent{Kind: "approval", Title: "Patch the web server", MaxTitle: 260}, barLevels(envelope(chimePCM("approval"), toneRate), 200, nBars)),
		mk(PillContent{Kind: "speech", Title: "Write the release notes", MaxTitle: 260, Muted: true}, pendingLevels(300, nBars)),
	}
	maxW := 0
	for _, f := range frames {
		if f.Bounds().Dx() > maxW {
			maxW = f.Bounds().Dx()
		}
	}
	fw, fh := maxW, 0
	for _, f := range frames {
		if f.Bounds().Dy() > fh {
			fh = f.Bounds().Dy()
		}
	}
	pad := 24
	sheet := image.NewRGBA(image.Rect(0, 0, fw+2*pad, len(frames)*(fh+pad)+pad))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{0x1f, 0x22, 0x36, 0xff}), image.Point{}, draw.Src)
	for i, f := range frames {
		r := image.Rect(pad, pad+i*(fh+pad), pad+f.Bounds().Dx(), pad+i*(fh+pad)+fh)
		draw.Draw(sheet, r, f, image.Point{}, draw.Over)
	}
	fo, _ := os.Create(out)
	defer fo.Close()
	png.Encode(fo, sheet)
	_, lay := renderOverlay(scale, pal, PillContent{Kind: "speech", Title: "VoiceLoop", MaxTitle: 260}, nil, 1)
	if lay.Mute.Empty() || lay.Stop.Empty() || lay.Mute.Overlaps(lay.Stop) {
		t.Fatalf("button areas wrong: %+v", lay)
	}
	if _, l2 := renderOverlay(scale, pal, PillContent{Kind: "speech"}, nil, 1); !l2.Mute.Empty() {
		t.Fatal("no title should mean no mute button")
	}
}

func TestChimes(t *testing.T) {
	for _, k := range []string{"question", "approval", "on", "off"} {
		p := chimePCM(k)
		if len(p) < 1000 {
			t.Fatalf("%s chime too short", k)
		}
		t.Logf("%s: %.2f s", k, float64(len(p)/2)/toneRate)
	}
}
