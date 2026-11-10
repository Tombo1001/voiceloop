package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testIcon is a small PNG: a black square on transparent, offset by shift so two calls
// with different shifts give different files.
func testIcon(t *testing.T, shift int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 16 + shift; y < 48; y++ {
		for x := 16; x < 48; x++ {
			img.Set(x, y, color.NRGBA{0, 0, 0, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestProfileCarriesCustomIcons(t *testing.T) {
	from, to := t.TempDir(), t.TempDir()
	custom := testIcon(t, 0)
	os.WriteFile(filepath.Join(from, "my-mic.png"), custom, 0o644)

	p, err := ParsePadProfile(defaultPadXML)
	if err != nil {
		t.Fatal(err)
	}
	p.Symbols = defaultSymbols()
	p.Symbols.Symbol = []PadSymbol{{Control: "key1", Icon: "my-mic.png"}, {Control: "key3", Icon: "send.png"}}
	p.embedIcons(from)
	xml := p.Marshal()
	if out := os.Getenv("PROFILE_SAMPLE"); out != "" { // for checking against pad-profile.xsd
		os.WriteFile(out, xml, 0o644)
	}
	if !bytes.Contains(xml, []byte(`icon="my-mic.png">`)) || !bytes.Contains(xml, []byte(`<symbol control="key3" icon="send.png"></symbol>`)) {
		t.Fatalf("only the custom icon should be embedded:\n%s", xml)
	}

	// Import on another PC: the icon is written to its icons folder.
	q, err := ParsePadProfile(xml)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := q.adoptEmbeddedIcons(to); err != nil || n != 1 {
		t.Fatalf("adopt: %d, %v", n, err)
	}
	if got, _ := os.ReadFile(filepath.Join(to, "my-mic.png")); !bytes.Equal(got, custom) {
		t.Fatal("icon not written, or changed")
	}
	if q.Symbols.Symbol[0].Data != "" || q.Symbols.Symbol[0].Icon != "my-mic.png" {
		t.Fatalf("after import: %+v", q.Symbols.Symbol[0])
	}
	if !strings.Contains(string(q.Marshal()), `<symbol control="key1" icon="my-mic.png"></symbol>`) {
		t.Fatal("the stored profile shouldn't keep the icon data")
	}

	// Importing the same profile again reuses the identical file.
	q, _ = ParsePadProfile(xml)
	if n, _ := q.adoptEmbeddedIcons(to); n != 0 || q.Symbols.Symbol[0].Icon != "my-mic.png" {
		t.Fatalf("re-import: %d, %+v", n, q.Symbols.Symbol[0])
	}

	// A different picture under the same name doesn't overwrite it.
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "my-mic.png"), testIcon(t, 4), 0o644)
	q, _ = ParsePadProfile(xml)
	if n, _ := q.adoptEmbeddedIcons(other); n != 1 || q.Symbols.Symbol[0].Icon != "my-mic-2.png" {
		t.Fatalf("name clash: %d, %+v", n, q.Symbols.Symbol[0])
	}

	// A custom file named like a built-in, but different, is embedded and lands beside it.
	os.WriteFile(filepath.Join(from, "dictate.png"), testIcon(t, 2), 0o644)
	p.Symbols.Symbol = []PadSymbol{{Control: "key1", Icon: "dictate.png"}}
	p.embedIcons(from)
	q, _ = ParsePadProfile(p.Marshal())
	if n, _ := q.adoptEmbeddedIcons(t.TempDir()); n != 1 || q.Symbols.Symbol[0].Icon != "dictate-2.png" {
		t.Fatalf("built-in name: %d, %+v", n, q.Symbols.Symbol[0])
	}
}

func TestProfileRejectsBadEmbeddedIcons(t *testing.T) {
	base := string(defaultPadXML)
	i, j := strings.Index(base, "<symbols"), strings.Index(base, "</symbols>")+len("</symbols>")
	for name, sym := range map[string]string{
		"bad base64":   `<symbol control="key1" icon="x.png">not base64!</symbol>`,
		"not an image": `<symbol control="key1" icon="x.png">aGVsbG8=</symbol>`,
	} {
		xml := base[:i] + `<symbols shape="square" size="9" layers="2" layerHeight="0.2">` + sym + `</symbols>` + base[j:]
		if _, err := ParsePadProfile([]byte(xml)); err == nil || !strings.Contains(err.Error(), "icon inside") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
