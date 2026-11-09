package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stlEdges checks a binary STL is watertight: every edge is shared by exactly
// two triangles, running opposite ways (consistent outward normals).
func stlEdges(t *testing.T, b []byte) int {
	t.Helper()
	n := int(binary.LittleEndian.Uint32(b[80:84]))
	if len(b) != 84+n*50 {
		t.Fatalf("STL size %d doesn't match %d triangles", len(b), n)
	}
	key := func(v [3]float32) string { return fmt.Sprintf("%.4f,%.4f,%.4f", v[0], v[1], v[2]) }
	edges := map[[2]string]int{}
	for i := 0; i < n; i++ {
		var tr struct {
			N [3]float32
			V [3][3]float32
		}
		binary.Read(bytes.NewReader(b[84+i*50:84+i*50+48]), binary.LittleEndian, &tr)
		for j := 0; j < 3; j++ {
			edges[[2]string{key(tr.V[j]), key(tr.V[(j+1)%3])}]++
		}
	}
	for e, c := range edges {
		if c != 1 || edges[[2]string{e[1], e[0]}] != 1 {
			t.Fatalf("edge %v used %d times, reverse %d: not watertight", e, c, edges[[2]string{e[1], e[0]}])
		}
	}
	return n
}

func TestBuiltinIconsPrint(t *testing.T) {
	entries, _ := builtinIcons.ReadDir("icons")
	if len(entries) < 8 {
		t.Fatalf("only %d built-in icons", len(entries))
	}
	for _, e := range entries {
		b, _ := builtinIcons.ReadFile("icons/" + e.Name())
		m, err := iconMask(b)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		for _, round := range []bool{false, true} {
			g := fitGrid(m, 9, round)
			if len(g) != 90 {
				t.Fatalf("%s: grid %d cells, want 90 for 9 mm", e.Name(), len(g))
			}
			// The artwork should touch the edge of the 9 mm box (square) or circle (round), not overflow.
			var maxR float64
			minX, maxX, minY, maxY := 90, -1, 90, -1
			for y := range g {
				for x, on := range g[y] {
					if on {
						minX, maxX, minY, maxY = min(minX, x), max(maxX, x), min(minY, y), max(maxY, y)
						maxR = math.Max(maxR, math.Hypot(float64(x)+0.5-45, float64(y)+0.5-45))
					}
				}
			}
			if round && (maxR > 45.01 || maxR < 42) {
				t.Errorf("%s round: furthest cell %.1f from centre, want about 45", e.Name(), maxR)
			}
			if span := max(maxX-minX, maxY-minY) + 1; !round && (span < 86 || span > 90) {
				t.Errorf("%s square: longer side %d cells, want about 90 (9 mm)", e.Name(), span)
			}
			stlEdges(t, symbolsSTL([]placedGrid{{g: g}}, 0.4))
		}
	}
}

func TestDefaultProfileSymbolsPlate(t *testing.T) {
	p, err := ParsePadProfile(defaultPadXML)
	if err != nil {
		t.Fatal(err)
	}
	stl, n, err := symbolsPlate(p, t.TempDir()) // empty folder: falls back to the built-ins
	if err != nil || n != 4 {
		t.Fatalf("plate: %d symbols, %v", n, err)
	}
	tris := stlEdges(t, stl)
	// Check the height and the footprint: 4 symbols of 9 mm, 3 mm apart, 0.4 mm thick.
	var maxX, maxY, maxZ float32
	for i := 0; i < tris; i++ {
		var v [12]float32
		binary.Read(bytes.NewReader(stl[84+i*50:84+i*50+48]), binary.LittleEndian, &v)
		for j := 3; j < 12; j += 3 {
			maxX, maxY, maxZ = max(maxX, v[j]), max(maxY, v[j+1]), max(maxZ, v[j+2])
		}
	}
	if math.Abs(float64(maxZ)-0.4) > 1e-4 || maxX > 4*9+3*3+0.01 || maxY > 9.01 {
		t.Fatalf("plate extents x %.2f y %.2f z %.2f", maxX, maxY, maxZ)
	}
}

func TestMatchSymbolsAndXML(t *testing.T) {
	p, _ := ParsePadProfile(defaultPadXML)
	p.Symbols.Symbol = nil
	_ = p.SetAction("key3", "ctrl-alt-v")
	if n := p.MatchSymbols(); n != 4 {
		t.Fatalf("matched %d", n)
	}
	if p.symbolIcon("key3") != "voice-mode.png" || p.symbolIcon("knob1") != "stop-speech.png" {
		t.Fatalf("%+v", p.Symbols.Symbol)
	}
	q, err := ParsePadProfile(p.Marshal())
	if err != nil || q.symbolIcon("key2") != "slash-command.png" || q.Symbols.Size != 9 {
		t.Fatalf("symbols lost in export: %v", err)
	}
	// Old profiles without <symbols> get the defaults.
	old := strings.Split(string(defaultPadXML), "<symbols")[0] + "</padProfile>\n"
	r, err := ParsePadProfile([]byte(old))
	if err != nil || r.Symbols == nil || r.Symbols.Shape != "square" || len(r.Symbols.Symbol) != 0 {
		t.Fatalf("old profile: %v %+v", err, r)
	}
	for _, bad := range []string{`icon="../evil.png"`, `icon="x.exe"`, `control="key9"`, `size="1"`} {
		from := map[bool]string{true: `control="key1"`, false: `icon="dictate.png"`}[strings.HasPrefix(bad, "control")]
		if strings.HasPrefix(bad, "size") {
			from = `size="9"`
		}
		if _, err := ParsePadProfile([]byte(strings.Replace(string(defaultPadXML), from, bad, 1))); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestAdoptIcon(t *testing.T) {
	icons, other := t.TempDir(), t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for i := 5; i < 15; i++ {
		img.Set(i, i, color.Black)
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	src := filepath.Join(other, "My Icon.png")
	os.WriteFile(src, b.Bytes(), 0o644)
	n, err := adoptIcon(icons, src)
	if err != nil || n != "My Icon.png" {
		t.Fatalf("%q %v", n, err)
	}
	if n2, _ := adoptIcon(icons, src); n2 != n {
		t.Fatalf("same file copied twice as %q", n2)
	}
	img.Set(1, 1, color.Black)
	b.Reset()
	png.Encode(&b, img)
	os.WriteFile(src, b.Bytes(), 0o644)
	if n3, _ := adoptIcon(icons, src); n3 != "My Icon-2.png" {
		t.Fatalf("different file with the same name became %q", n3)
	}
	if n4, err := adoptIcon(icons, filepath.Join(icons, "My Icon.png")); err != nil || n4 != "My Icon.png" {
		t.Fatalf("file already in the folder: %q %v", n4, err)
	}
	os.WriteFile(filepath.Join(other, "notes.txt"), []byte("hi"), 0o644)
	if _, err := adoptIcon(icons, filepath.Join(other, "notes.txt")); err == nil {
		t.Fatal("a text file was accepted as an icon")
	}
}
