package logo

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

// Contact sheet on a dark and a light strip, at real icon sizes, when $PREVIEW is set.
func TestPreview(t *testing.T) {
	out := os.Getenv("PREVIEW")
	if out == "" {
		t.Skip("set PREVIEW")
	}
	sizes := []int{16, 20, 24, 32, 48, 64, 128, 256}
	w := 24
	for _, s := range sizes {
		w += s + 24
	}
	sheet := image.NewRGBA(image.Rect(0, 0, w, 2*(256+40)+256+40))
	bands := []color.RGBA{{0x20, 0x20, 0x20, 0xff}, {0xee, 0xee, 0xee, 0xff}, {0x10, 0x3d, 0x22, 0xff}}
	for b, bg := range bands {
		r := image.Rect(0, b*(256+40), w, (b+1)*(256+40))
		draw.Draw(sheet, r, image.NewUniform(bg), image.Point{}, draw.Src)
		x := 24
		for i, s := range sizes {
			col := Teal
			if b == 2 && i%2 == 1 {
				col = Grey // off state in the tray
			}
			y := b*(256+40) + 20 + (256-s)/2
			draw.Draw(sheet, image.Rect(x, y, x+s, y+s), Render(s, col), image.Point{}, draw.Over)
			x += s + 24
		}
	}
	f, _ := os.Create(out)
	defer f.Close()
	png.Encode(f, sheet)
	if len(ICO(Teal, 16, 32, 256)) < 500 {
		t.Fatal("ico too small")
	}
}
