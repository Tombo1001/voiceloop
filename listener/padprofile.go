package main

// Pad profiles: one XML file per macropad model, holding how to find it on USB,
// which protocol it speaks, its keys, knobs and light modes, and the user's key
// map. The active profile lives next to the exe as pad-profile.xml. Import and
// reset save the old one first, dated, in pad-profiles\. Schema: pads/pad-profile.xsd.

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed pads/ch57x-3key-knob.xml
var defaultPadXML []byte

// padNS is the profile namespace. Profiles written before 0.8.0 use padNSOld, a
// placeholder from before the project had a home; they still load.
const (
	padNS    = "https://voiceloop.dev/pad-profile/1"
	padNSOld = "https://github.com/voiceloop/pad-profile/1"
)

type PadProfile struct {
	XMLName     xml.Name    `xml:"padProfile" json:"-"`
	NS          string      `xml:"xmlns,attr" json:"-"`
	Version     int         `xml:"version,attr" json:"version"`
	Name        string      `xml:"name" json:"name"`
	Description string      `xml:"description,omitempty" json:"description,omitempty"`
	Link        string      `xml:"link,omitempty" json:"link,omitempty"`
	USB         PadUSB      `xml:"usb" json:"usb"`
	Protocol    string      `xml:"protocol" json:"protocol"`
	Limits      PadLimits   `xml:"limits" json:"limits"`
	Keys        PadKeys     `xml:"keys" json:"keys"`
	Knobs       []PadKnob   `xml:"knobs>knob" json:"knobs"`
	Lights      []PadLight  `xml:"lights>mode" json:"lights"`
	Mapping     PadMapping  `xml:"mapping" json:"mapping"`
	Symbols     *PadSymbols `xml:"symbols,omitempty" json:"symbols"` // 3D-printed key symbols (optional)
}

type PadUSB struct {
	VendorID     string `xml:"vendorId,attr" json:"vendor_id"`
	ProductID    string `xml:"productId,attr" json:"product_id"`
	UsagePage    string `xml:"usagePage,attr" json:"usage_page"`
	ReportLength int    `xml:"reportLength,attr" json:"report_length"`
}

type PadLimits struct {
	MaxSequence        int  `xml:"maxSequence,attr" json:"max_sequence"`
	ModifiersFirstOnly bool `xml:"modifiersOnFirstKeyOnly,attr" json:"modifiers_first_only"`
}

type PadKeys struct {
	Rows    int      `xml:"rows,attr" json:"rows"`
	Columns int      `xml:"columns,attr" json:"columns"`
	Key     []PadKey `xml:"key" json:"list"`
}

type PadKey struct {
	ID    string `xml:"id,attr" json:"id"`
	Code  int    `xml:"code,attr" json:"code"`
	Label string `xml:"label,attr,omitempty" json:"label,omitempty"`
}

type PadKnob struct {
	ID        string `xml:"id,attr" json:"id"`
	TurnLeft  int    `xml:"turnLeft,attr" json:"turn_left"`
	Press     int    `xml:"press,attr" json:"press"`
	TurnRight int    `xml:"turnRight,attr" json:"turn_right"`
	Label     string `xml:"label,attr,omitempty" json:"label,omitempty"`
}

type PadLight struct {
	Value int    `xml:"value,attr" json:"value"`
	Name  string `xml:",chardata" json:"name"`
}

type PadMapping struct {
	KnobSide string    `xml:"knobSide,attr" json:"knob_side"` // "right" (normal) or "left" (pad turned round)
	Lights   int       `xml:"lights,attr" json:"lights"`
	Bind     []PadBind `xml:"bind" json:"bind"`
}

type PadBind struct {
	Control string `xml:"control,attr" json:"control"`
	Action  string `xml:"action,attr" json:"action"`
}

var padIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func parseHex16(s string) (uint16, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 0, 16)
	return uint16(v), err
}

// ParsePadProfile reads and checks a profile. Errors are written for the person
// who made the file, so they say what to fix.
func ParsePadProfile(b []byte) (*PadProfile, error) {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var p PadProfile
	if err := xml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("not a pad profile: %v", err)
	}
	if p.XMLName.Local != "padProfile" {
		return nil, fmt.Errorf("not a pad profile: the root element should be <padProfile>")
	}
	if ns := p.XMLName.Space; ns != "" && ns != padNS && ns != padNSOld {
		return nil, fmt.Errorf("not a VoiceLoop pad profile: its namespace is %q, expected %q", ns, padNS)
	}
	if p.Version != 1 {
		return nil, fmt.Errorf("profile version %d isn't supported by this VoiceLoop (it reads version 1)", p.Version)
	}
	if err := p.check(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *PadProfile) check() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("<name> is missing")
	}
	for what, s := range map[string]string{"vendorId": p.USB.VendorID, "productId": p.USB.ProductID, "usagePage": p.USB.UsagePage} {
		if _, err := parseHex16(s); err != nil {
			return fmt.Errorf("<usb %s=%q> should be a hex number like 0x1189", what, s)
		}
	}
	if p.USB.ReportLength < 2 || p.USB.ReportLength > 1024 {
		return fmt.Errorf("<usb reportLength> should be the report size in bytes, including the report ID")
	}
	if _, ok := padEncoders[p.Protocol]; !ok {
		return fmt.Errorf("this VoiceLoop doesn't know the protocol %q (it knows: %s)", p.Protocol, strings.Join(padEncoderNames(), ", "))
	}
	if p.Limits.MaxSequence < 1 {
		return fmt.Errorf("<limits maxSequence> should be at least 1")
	}
	if len(p.Keys.Key) == 0 && len(p.Knobs) == 0 {
		return fmt.Errorf("the profile has no keys or knobs")
	}
	if p.Keys.Rows*p.Keys.Columns != len(p.Keys.Key) {
		return fmt.Errorf("<keys rows=%d columns=%d> means %d keys, but %d are listed", p.Keys.Rows, p.Keys.Columns, p.Keys.Rows*p.Keys.Columns, len(p.Keys.Key))
	}
	controls := map[string]bool{}
	codes := map[int]string{}
	addCode := func(c int, who string) error {
		if c < 1 || c > 255 {
			return fmt.Errorf("%s: code %d should be 1-255", who, c)
		}
		if o, dup := codes[c]; dup {
			return fmt.Errorf("%s and %s both use code %d", o, who, c)
		}
		codes[c] = who
		return nil
	}
	for _, k := range p.Keys.Key {
		if !padIDRe.MatchString(k.ID) || controls[k.ID] {
			return fmt.Errorf("key id %q should be unique, lower case, like key1", k.ID)
		}
		controls[k.ID] = true
		if err := addCode(k.Code, k.ID); err != nil {
			return err
		}
	}
	for _, k := range p.Knobs {
		if !padIDRe.MatchString(k.ID) || controls[k.ID] {
			return fmt.Errorf("knob id %q should be unique, lower case, like knob1", k.ID)
		}
		controls[k.ID] = true
		for _, part := range []struct {
			s string
			c int
		}{{".left", k.TurnLeft}, {".press", k.Press}, {".right", k.TurnRight}} {
			controls[k.ID+part.s] = true
			if err := addCode(part.c, k.ID+part.s); err != nil {
				return err
			}
		}
	}
	lights := map[int]bool{}
	for _, l := range p.Lights {
		if l.Value < 0 || l.Value > 255 {
			return fmt.Errorf("light mode %d should be 0-255", l.Value)
		}
		lights[l.Value] = true
	}
	m := &p.Mapping
	if m.KnobSide == "" {
		m.KnobSide = "right"
	}
	if m.KnobSide != "right" && m.KnobSide != "left" {
		return fmt.Errorf(`<mapping knobSide> should be "right" or "left"`)
	}
	if len(p.Lights) > 0 && !lights[m.Lights] {
		return fmt.Errorf("<mapping lights=%d> isn't one of the listed light modes", m.Lights)
	}
	if p.Symbols == nil {
		p.Symbols = defaultSymbols() // profiles from before v0.7 have no <symbols>
	}
	if err := p.Symbols.check(p); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, b := range m.Bind {
		if !controls[b.Control] || isKnobOnly(p, b.Control) {
			return fmt.Errorf("<bind control=%q>: no such key or knob action (knobs use knob1.left, knob1.press, knob1.right)", b.Control)
		}
		if seen[b.Control] {
			return fmt.Errorf("%s is bound twice", b.Control)
		}
		seen[b.Control] = true
		if _, err := p.parseAction(b.Action); err != nil {
			return fmt.Errorf("%s: %v", b.Control, err)
		}
	}
	return nil
}

// isKnobOnly is true for a bare knob id, which can't be bound (only its .left/.press/.right).
func isKnobOnly(p *PadProfile, id string) bool {
	for _, k := range p.Knobs {
		if k.ID == id {
			return true
		}
	}
	return false
}

func (p *PadProfile) parseAction(s string) (padAction, error) {
	return parsePadActionLimits(s, p.Limits.MaxSequence, p.Limits.ModifiersFirstOnly)
}

// Action returns what a control is bound to ("" if unbound).
func (p *PadProfile) Action(control string) string {
	for _, b := range p.Mapping.Bind {
		if b.Control == control {
			return b.Action
		}
	}
	return ""
}

// SetAction binds a control, keeping the bind list in the profile's control order.
func (p *PadProfile) SetAction(control, action string) error {
	if _, err := p.parseAction(action); err != nil {
		return err
	}
	m := map[string]string{}
	for _, b := range p.Mapping.Bind {
		m[b.Control] = b.Action
	}
	m[control] = action
	p.Mapping.Bind = nil
	for _, c := range p.ControlOrder() {
		if a, ok := m[c]; ok {
			p.Mapping.Bind = append(p.Mapping.Bind, PadBind{c, a})
		}
	}
	return p.check()
}

// ControlOrder lists every bindable control: keys, then each knob's left/press/right.
func (p *PadProfile) ControlOrder() []string {
	var out []string
	for _, k := range p.Keys.Key {
		out = append(out, k.ID)
	}
	for _, k := range p.Knobs {
		out = append(out, k.ID+".left", k.ID+".press", k.ID+".right")
	}
	return out
}

// Marshal writes the profile as indented XML with a header comment.
func (p *PadProfile) Marshal() []byte {
	p.NS = padNS
	b, _ := xml.MarshalIndent(p, "", "  ")
	head := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<!-- VoiceLoop pad profile. Import it from Settings > Macropad. Schema: pad-profile.xsd -->\n"
	return append(append([]byte(head), b...), '\n')
}

// ---------- encoders ----------

type padEncoder func(p *PadProfile) ([][]byte, error)
type padLightEncoder func(mode int) [][]byte

var padEncoders = map[string]struct {
	keys   padEncoder
	lights padLightEncoder
}{
	"ch57x-8890": {encode8890, padLightMessages},
}

func padEncoderNames() []string {
	var n []string
	for k := range padEncoders {
		n = append(n, k)
	}
	return n
}

// padMessagesFor turns the profile's key map into the reports to send.
func padMessagesFor(p *PadProfile) ([][]byte, error) {
	e := padEncoders[p.Protocol]
	msgs, err := e.keys(p)
	if err != nil {
		return nil, err
	}
	if len(p.Lights) > 0 {
		msgs = append(msgs, e.lights(p.Mapping.Lights)...)
	}
	return msgs, nil
}

// physicalCodes maps each control to the code it has on the pad, allowing for
// a pad turned round: then the key grid and the knob order are reversed.
func (p *PadProfile) physicalCodes() map[string]int {
	out := map[string]int{}
	keys, knobs := p.Keys.Key, p.Knobs
	flip := p.Mapping.KnobSide == "left"
	for i, k := range keys {
		src := k
		if flip {
			src = keys[len(keys)-1-i]
		}
		out[k.ID] = src.Code
	}
	for i, k := range knobs {
		src := k
		if flip {
			src = knobs[len(knobs)-1-i]
		}
		out[k.ID+".left"], out[k.ID+".press"], out[k.ID+".right"] = src.TurnLeft, src.Press, src.TurnRight
	}
	return out
}

func encode8890(p *PadProfile) ([][]byte, error) {
	out := [][]byte{make([]byte, 64)}
	codes := p.physicalCodes()
	for _, c := range p.ControlOrder() {
		a, err := p.parseAction(p.Action(c))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c, err)
		}
		padBind(&out, byte(codes[c]), a)
	}
	return out, nil
}

// ---------- the active profile ----------

type padStore struct {
	mu   sync.Mutex
	dir  string
	prof *PadProfile
}

var pads padStore

func (s *padStore) path() string { return filepath.Join(s.dir, "pad-profile.xml") }

// load reads pad-profile.xml, or makes it from the built-in default, carrying
// over a key map saved by VoiceLoop 0.5.x in settings.json.
func (s *padStore) load(dir string, legacy *PadMap) (migrated bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dir = dir
	if b, rerr := os.ReadFile(s.path()); rerr == nil {
		p, perr := ParsePadProfile(b)
		if perr == nil {
			s.prof = p
			return false, nil
		}
		err = fmt.Errorf("pad-profile.xml unreadable, using the built-in pad: %v", perr)
		s.backupLocked("unreadable")
	}
	p, _ := ParsePadProfile(defaultPadXML)
	if legacy != nil {
		migrated = true
		for i, a := range legacy.Keys {
			_ = p.SetAction(fmt.Sprintf("key%d", i+1), a)
		}
		_ = p.SetAction("knob1.left", legacy.KnobCCW)
		_ = p.SetAction("knob1.press", legacy.KnobPress)
		_ = p.SetAction("knob1.right", legacy.KnobCW)
		if legacy.KnobLeft {
			p.Mapping.KnobSide = "left"
		}
		p.Mapping.Lights = legacy.Lights
		if p.check() != nil {
			p, _ = ParsePadProfile(defaultPadXML)
		}
	}
	s.prof = p
	if werr := s.saveLocked(); werr != nil && err == nil {
		err = werr
	}
	return migrated, err
}

func (s *padStore) get() *PadProfile {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *s.prof
	c.Keys.Key = append([]PadKey(nil), s.prof.Keys.Key...)
	c.Knobs = append([]PadKnob(nil), s.prof.Knobs...)
	c.Lights = append([]PadLight(nil), s.prof.Lights...)
	c.Mapping.Bind = append([]PadBind(nil), s.prof.Mapping.Bind...)
	if s.prof.Symbols != nil {
		sy := *s.prof.Symbols
		sy.Symbol = append([]PadSymbol(nil), s.prof.Symbols.Symbol...)
		c.Symbols = &sy
	}
	return &c
}

// update applies f to a copy of the profile and saves it if f and the checks pass.
func (s *padStore) update(f func(p *PadProfile) error) (*PadProfile, error) {
	p := s.get()
	if err := f(p); err != nil {
		return nil, err
	}
	if err := p.check(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prof = p
	return p, s.saveLocked()
}

// replace swaps in a new profile (import or reset), saving the old one first.
// It returns the backup's file name.
func (s *padStore) replace(p *PadProfile, why string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, err := s.backupLocked(why)
	if err != nil {
		return "", fmt.Errorf("couldn't save a copy of the current pad first, so nothing was changed: %v", err)
	}
	s.prof = p
	return name, s.saveLocked()
}

func (s *padStore) saveLocked() error {
	if s.dir == "" {
		return nil
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, s.prof.Marshal(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

// backupLocked copies the current pad-profile.xml into pad-profiles\ with the date and time.
func (s *padStore) backupLocked(why string) (string, error) {
	if s.dir == "" {
		return "", nil
	}
	b, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	dir := filepath.Join(s.dir, "pad-profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("pad-profile-%s-before-%s.xml", time.Now().Format("2006-01-02-150405"), why)
	return name, os.WriteFile(filepath.Join(dir, name), b, 0o644)
}
