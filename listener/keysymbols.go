package main

// Key symbols: turn icon images into flat, printable shapes to glue onto the
// pad's keys. An icon is any PNG or JPEG: transparent background with dark
// artwork, or dark artwork on white. It's scaled to fit the key (square or
// round, size in mm), sampled onto a 0.1 mm grid, and extruded to N layers.
// The mesh is built from grid cells with shared vertices, so it's watertight:
// slicers take it as it is.

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed icons/*.png
var builtinIcons embed.FS

const symbolPitch = 0.1 // mm per grid cell

// PadSymbols is the <symbols> part of a pad profile.
type PadSymbols struct {
	Shape       string      `xml:"shape,attr" json:"shape"` // "square" or "round", for keys; knob caps are always round
	Size        float64     `xml:"size,attr" json:"size"`   // mm: side of the square or diameter of the circle
	Layers      int         `xml:"layers,attr" json:"layers"`
	LayerHeight float64     `xml:"layerHeight,attr" json:"layer_height"`
	Symbol      []PadSymbol `xml:"symbol" json:"list"`
}

type PadSymbol struct {
	Control string `xml:"control,attr" json:"control"` // a key id, or a knob id for its cap
	Icon    string `xml:"icon,attr" json:"icon"`       // file name in the icons folder
	// Data is the icon itself, base64, in exported profiles only, for icons that aren't built
	// in. Import writes it to the icons folder and clears it.
	Data string `xml:",chardata" json:"-"`
}

const maxEmbeddedIcon = 1 << 20 // bytes, per icon

func defaultSymbols() *PadSymbols {
	return &PadSymbols{Shape: "square", Size: 9, Layers: 2, LayerHeight: 0.2}
}

func (s *PadSymbols) check(p *PadProfile) error {
	if s.Shape != "square" && s.Shape != "round" {
		return fmt.Errorf(`<symbols shape> should be "square" or "round"`)
	}
	if s.Size < 3 || s.Size > 60 {
		return fmt.Errorf("<symbols size> should be 3-60 mm")
	}
	if s.Layers < 1 || s.Layers > 20 {
		return fmt.Errorf("<symbols layers> should be 1-20")
	}
	if s.LayerHeight < 0.04 || s.LayerHeight > 0.6 {
		return fmt.Errorf("<symbols layerHeight> should be 0.04-0.6 mm")
	}
	ok := map[string]bool{}
	for _, k := range p.Keys.Key {
		ok[k.ID] = true
	}
	for _, k := range p.Knobs {
		ok[k.ID] = true
	}
	seen := map[string]bool{}
	for _, x := range s.Symbol {
		if !ok[x.Control] {
			return fmt.Errorf("<symbol control=%q>: no such key or knob", x.Control)
		}
		if seen[x.Control] {
			return fmt.Errorf("%s has two symbols", x.Control)
		}
		seen[x.Control] = true
		if !safeIconName(x.Icon) {
			return fmt.Errorf("<symbol icon=%q> should be a file name in the icons folder, like dictate.png", x.Icon)
		}
		if strings.TrimSpace(x.Data) != "" {
			b, err := base64.StdEncoding.DecodeString(stripSpace(x.Data))
			if err != nil {
				return fmt.Errorf("the icon inside <symbol icon=%q> isn't valid base64", x.Icon)
			}
			if len(b) > maxEmbeddedIcon {
				return fmt.Errorf("the icon inside <symbol icon=%q> is over 1 MB", x.Icon)
			}
			if _, err := iconMask(b); err != nil {
				return fmt.Errorf("the icon inside <symbol icon=%q>: %v", x.Icon, err)
			}
		}
	}
	return nil
}

func safeIconName(n string) bool {
	if n == "" || n != filepath.Base(n) || strings.ContainsAny(n, `\/:*?"<>|`) || strings.HasPrefix(n, ".") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(n))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg"
}

// isRound says whether a control's symbol is fitted to a circle.
func (p *PadProfile) isRound(control string) bool {
	for _, k := range p.Knobs {
		if k.ID == control {
			return true
		}
	}
	return p.Symbols.Shape == "round"
}

func (p *PadProfile) symbolIcon(control string) string {
	if p.Symbols == nil {
		return ""
	}
	for _, x := range p.Symbols.Symbol {
		if x.Control == control {
			return x.Icon
		}
	}
	return ""
}

// SetSymbol puts an icon on a control ("" clears it), keeping profile order.
func (p *PadProfile) SetSymbol(control, icon string) {
	if p.Symbols == nil {
		p.Symbols = defaultSymbols()
	}
	m := map[string]string{}
	for _, x := range p.Symbols.Symbol {
		m[x.Control] = x.Icon
	}
	if icon == "" {
		delete(m, control)
	} else {
		m[control] = icon
	}
	p.Symbols.Symbol = nil
	for _, c := range p.symbolControls() {
		if m[c] != "" {
			p.Symbols.Symbol = append(p.Symbols.Symbol, PadSymbol{Control: c, Icon: m[c]})
		}
	}
}

// symbolControls lists what can carry a symbol: every key, then each knob cap.
func (p *PadProfile) symbolControls() []string {
	var out []string
	for _, k := range p.Keys.Key {
		out = append(out, k.ID)
	}
	for _, k := range p.Knobs {
		out = append(out, k.ID)
	}
	return out
}

// ---------- icons to grids ----------

// iconMask decodes an image and marks the artwork: opaque pixels when the
// image has transparency, otherwise dark pixels.
func iconMask(b []byte) ([][]bool, error) {
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("not a PNG or JPEG image: %v", err)
	}
	r := img.Bounds()
	hasAlpha := false
	for y := r.Min.Y; y < r.Max.Y && !hasAlpha; y += 4 {
		for x := r.Min.X; x < r.Max.X; x += 4 {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xf000 {
				hasAlpha = true
				break
			}
		}
	}
	m := make([][]bool, r.Dy())
	any := false
	for y := range m {
		m[y] = make([]bool, r.Dx())
		for x := range m[y] {
			c := color.NRGBAModel.Convert(img.At(r.Min.X+x, r.Min.Y+y)).(color.NRGBA)
			lum := (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
			on := lum < 128
			if hasAlpha {
				on = c.A > 128 && lum < 200
			}
			m[y][x] = on
			any = any || on
		}
	}
	if !any {
		return nil, fmt.Errorf("the image has no dark or opaque artwork to print")
	}
	return m, nil
}

// fitGrid scales the artwork to fit size mm (a square, or a circle when round)
// and samples it onto a grid of symbolPitch cells. The grid is size x size mm,
// with the artwork centred.
func fitGrid(m [][]bool, size float64, round bool) [][]bool {
	h, w := len(m), len(m[0])
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := range m {
		for x, on := range m[y] {
			if on {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	bw, bh := float64(maxX-minX+1), float64(maxY-minY+1)
	cx, cy := float64(minX)+bw/2, float64(minY)+bh/2
	// Source pixels per mm. Square: the longer side fills the square.
	// Round: the artwork pixel furthest from the centre sits on the circle.
	scale := math.Max(bw, bh) / size
	if round {
		far := 0.0
		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				if m[y][x] {
					for _, d := range [][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
						far = math.Max(far, math.Hypot(float64(x)+d[0]-cx, float64(y)+d[1]-cy))
					}
				}
			}
		}
		scale = far / (size / 2)
	}
	n := int(math.Round(size / symbolPitch))
	g := make([][]bool, n)
	for gy := range g {
		g[gy] = make([]bool, n)
		for gx := range g[gy] {
			// Centre of this cell in mm from the grid centre, back to source pixels.
			mx := (float64(gx)+0.5)*symbolPitch - size/2
			my := (float64(gy)+0.5)*symbolPitch - size/2
			sx, sy := int(math.Floor(cx+mx*scale)), int(math.Floor(cy+my*scale))
			if sx >= 0 && sy >= 0 && sx < w && sy < h {
				g[gy][gx] = m[sy][sx]
			}
		}
	}
	return g
}

// gridPNG draws a grid as a white-on-transparent PNG, for the preview.
func gridPNG(g [][]bool) []byte {
	n := len(g)
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := range g {
		for x, on := range g[y] {
			if on {
				img.SetNRGBA(x, y, color.NRGBA{240, 240, 242, 255})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// ---------- grids to STL ----------

type placedGrid struct {
	g      [][]bool
	ox, oy float64 // mm, the grid's top-left corner on the build plate
}

// symbolsSTL extrudes the grids to thick mm and returns a binary STL. Image
// y runs down, the plate's y runs up, so rows are flipped to keep the symbol
// the right way round when looked at from above.
func symbolsSTL(grids []placedGrid, thick float64) []byte {
	type tri [3][3]float32
	var tris []tri
	add := func(a, b, c [3]float64) {
		tris = append(tris, tri{{float32(a[0]), float32(a[1]), float32(a[2])}, {float32(b[0]), float32(b[1]), float32(b[2])}, {float32(c[0]), float32(c[1]), float32(c[2])}})
	}
	quad := func(a, b, c, d [3]float64) { add(a, b, c); add(a, c, d) } // counter-clockwise seen from outside
	for _, pg := range grids {
		n := len(pg.g)
		on := func(x, y int) bool { return x >= 0 && y >= 0 && y < n && x < len(pg.g[y]) && pg.g[y][x] }
		p := func(x, y int, z float64) [3]float64 {
			return [3]float64{pg.ox + float64(x)*symbolPitch, pg.oy + float64(n-y)*symbolPitch, z}
		}
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				if !on(x, y) {
					continue
				}
				// Corners: (x,y) top-left in image terms = high plate y.
				tl, tr, br, bl := func(z float64) [3]float64 { return p(x, y, z) }, func(z float64) [3]float64 { return p(x+1, y, z) },
					func(z float64) [3]float64 { return p(x+1, y+1, z) }, func(z float64) [3]float64 { return p(x, y+1, z) }
				quad(bl(thick), br(thick), tr(thick), tl(thick)) // top, normal +z
				quad(bl(0), tl(0), tr(0), br(0))                 // bottom, normal -z
				if !on(x, y-1) {                                 // edge at high plate y, normal +y
					quad(tr(0), tl(0), tl(thick), tr(thick))
				}
				if !on(x, y+1) { // normal -y
					quad(bl(0), br(0), br(thick), bl(thick))
				}
				if !on(x-1, y) { // normal -x
					quad(tl(0), bl(0), bl(thick), tl(thick))
				}
				if !on(x+1, y) { // normal +x
					quad(br(0), tr(0), tr(thick), br(thick))
				}
			}
		}
	}
	var b bytes.Buffer
	head := make([]byte, 80)
	copy(head, "voiceloop key symbols")
	b.Write(head)
	binary.Write(&b, binary.LittleEndian, uint32(len(tris)))
	for _, t := range tris {
		u := [3]float32{t[1][0] - t[0][0], t[1][1] - t[0][1], t[1][2] - t[0][2]}
		v := [3]float32{t[2][0] - t[0][0], t[2][1] - t[0][1], t[2][2] - t[0][2]}
		nx, ny, nz := u[1]*v[2]-u[2]*v[1], u[2]*v[0]-u[0]*v[2], u[0]*v[1]-u[1]*v[0]
		l := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
		if l > 0 {
			nx, ny, nz = nx/l, ny/l, nz/l
		}
		binary.Write(&b, binary.LittleEndian, [3]float32{nx, ny, nz})
		binary.Write(&b, binary.LittleEndian, t)
		b.Write([]byte{0, 0})
	}
	return b.Bytes()
}

// ---------- the icons folder ----------

// installIcons copies the built-in icons into dir, leaving any file already
// there alone (the user may have edited it).
func installIcons(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, _ := builtinIcons.ReadDir("icons")
	for _, e := range entries {
		dst := filepath.Join(dir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, _ := builtinIcons.ReadFile("icons/" + e.Name())
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
}

// embedIcons puts each custom icon into the profile, so an exported profile carries its
// symbols to another PC. Built-in icons stay as names, unless the icons folder holds a
// different file under a built-in's name.
func (p *PadProfile) embedIcons(dir string) {
	if p.Symbols == nil {
		return
	}
	for i := range p.Symbols.Symbol {
		x := &p.Symbols.Symbol[i]
		x.Data = ""
		b, err := os.ReadFile(filepath.Join(dir, x.Icon))
		if err != nil || len(b) > maxEmbeddedIcon {
			continue // a built-in, or missing: the name is all there is
		}
		if bi, err := builtinIcons.ReadFile("icons/" + x.Icon); err == nil && bytes.Equal(bi, b) {
			continue
		}
		x.Data = base64.StdEncoding.EncodeToString(b)
	}
}

// adoptEmbeddedIcons writes icons carried in an imported profile to the icons folder and
// clears them from the profile. A file already there under the same name is reused when
// it's identical; otherwise the icon is saved as name-2.png (or -3...) and renamed in the
// profile. It returns how many icons it wrote.
func (p *PadProfile) adoptEmbeddedIcons(dir string) (int, error) {
	if p.Symbols == nil {
		return 0, nil
	}
	n := 0
	for i := range p.Symbols.Symbol {
		x := &p.Symbols.Symbol[i]
		data := stripSpace(x.Data)
		x.Data = ""
		if data == "" {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return n, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return n, err
		}
		ext := filepath.Ext(x.Icon)
		stem := strings.TrimSuffix(x.Icon, ext)
		for k := 1; ; k++ {
			name := x.Icon
			if k > 1 {
				name = fmt.Sprintf("%s-%d%s", stem, k, ext)
			}
			old, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil && !os.IsNotExist(err) {
				return n, err
			}
			if err == nil && bytes.Equal(old, b) {
				x.Icon = name
				break
			}
			if os.IsNotExist(err) {
				if bi, err := builtinIcons.ReadFile("icons/" + name); err == nil && !bytes.Equal(bi, b) {
					continue // don't shadow a built-in with a different picture
				}
				if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
					return n, err
				}
				x.Icon = name
				n++
				break
			}
		}
	}
	return n, nil
}

// readIcon loads an icon by name: the icons folder first, then the built-ins.
func readIcon(dir, name string) ([]byte, error) {
	if !safeIconName(name) {
		return nil, fmt.Errorf("bad icon name %q", name)
	}
	if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
		return b, nil
	}
	if b, err := builtinIcons.ReadFile("icons/" + name); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("icon %s isn't in the icons folder", name)
}

// adoptIcon makes a picked file usable by name: inside the icons folder it's
// used as is, otherwise it's copied in (with -2, -3... if the name is taken
// by a different file).
func adoptIcon(dir, path string) (string, error) {
	abs, _ := filepath.Abs(path)
	if strings.EqualFold(filepath.Dir(abs), filepath.Clean(dir)) {
		if !safeIconName(filepath.Base(abs)) {
			return "", fmt.Errorf("icons have to be .png or .jpg files")
		}
		return filepath.Base(abs), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if _, err := iconMask(b); err != nil {
		return "", err
	}
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return "", fmt.Errorf("icons have to be .png or .jpg files")
	}
	stem := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSuffix(base, filepath.Ext(base)))
	stem = strings.TrimLeft(stem, ".")
	if stem == "" {
		stem = "icon"
	}
	for i := 1; ; i++ {
		name := stem + ext
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		dst := filepath.Join(dir, name)
		old, err := os.ReadFile(dst)
		if os.IsNotExist(err) {
			return name, os.WriteFile(dst, b, 0o644)
		}
		if err == nil && bytes.Equal(old, b) {
			return name, nil
		}
	}
}

// symbolGrid loads a control's icon and fits it.
func symbolGrid(p *PadProfile, dir, control string) ([][]bool, error) {
	name := p.symbolIcon(control)
	if name == "" {
		return nil, nil
	}
	b, err := readIcon(dir, name)
	if err != nil {
		return nil, err
	}
	m, err := iconMask(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	return fitGrid(m, p.Symbols.Size, p.isRound(control)), nil
}

// symbolsPlate lays every symbol out in a row, 3 mm apart, in control order.
func symbolsPlate(p *PadProfile, dir string) ([]byte, int, error) {
	if p.Symbols == nil || len(p.Symbols.Symbol) == 0 {
		return nil, 0, fmt.Errorf("no symbols chosen yet")
	}
	var grids []placedGrid
	x := 0.0
	ctl := p.symbolControls()
	order := map[string]int{}
	for i, c := range ctl {
		order[c] = i
	}
	list := append([]PadSymbol(nil), p.Symbols.Symbol...)
	sort.Slice(list, func(i, j int) bool { return order[list[i].Control] < order[list[j].Control] })
	for _, s := range list {
		g, err := symbolGrid(p, dir, s.Control)
		if err != nil {
			return nil, 0, err
		}
		grids = append(grids, placedGrid{g: g, ox: x, oy: 0})
		x += p.Symbols.Size + 3
	}
	return symbolsSTL(grids, float64(p.Symbols.Layers)*p.Symbols.LayerHeight), len(grids), nil
}

// actionIcons maps voiceloop's pad actions to the starter icons.
var actionIcons = map[string]string{
	"ctrl-space": "dictate.png", "ctrl-space,slash": "slash-command.png", "enter": "send.png",
	"ctrl-alt-x": "stop-speech.png", "ctrl-alt-v": "voice-mode.png", "mute": "mute.png",
	"play": "play-pause.png", "escape": "esc.png", "volumeup": "volume.png", "volumedown": "volume.png",
}

// MatchSymbols gives each key (and knob cap, by its press action) the starter
// icon for what it does. Controls with no matching icon keep what they have.
func (p *PadProfile) MatchSymbols() int {
	n := 0
	for _, k := range p.Keys.Key {
		if ic := iconFor(p.Action(k.ID)); ic != "" {
			p.SetSymbol(k.ID, ic)
			n++
		}
	}
	for _, k := range p.Knobs {
		if ic := iconFor(p.Action(k.ID + ".press")); ic != "" {
			p.SetSymbol(k.ID, ic)
			n++
		}
	}
	return n
}
