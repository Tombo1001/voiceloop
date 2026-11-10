package main

// Macropad actions and the ch57x-8890 protocol. Which pad, which codes and the
// key map come from the pad profile (padprofile.go). The protocol is ported from
// ch57x-keyboard-tool (kriomant, MIT/Apache-2.0), src/keyboard/k8890.rs. The key
// map is stored on the pad itself, so it works with or without voiceloop running.

import (
	"fmt"
	"strings"
)

// PadMap is the key map as voiceloop 0.5.x kept it in settings.json. It's only
// read once, to carry it over into pad-profile.xml.
type PadMap struct {
	Keys      [3]string `json:"keys"`
	KnobCCW   string    `json:"knob_ccw"`
	KnobPress string    `json:"knob_press"`
	KnobCW    string    `json:"knob_cw"`
	KnobLeft  bool      `json:"knob_left"`
	Lights    int       `json:"lights"`
}

var padModifiers = map[string]byte{
	"ctrl": 0x01, "shift": 0x02, "alt": 0x04, "win": 0x08,
	"rctrl": 0x10, "rshift": 0x20, "ralt": 0x40, "rwin": 0x80,
}

// HID consumer-page codes for the media keys.
var padMedia = map[string]uint16{
	"volumeup": 0xe9, "volumedown": 0xea, "mute": 0xe2,
	"play": 0xcd, "next": 0xb5, "previous": 0xb6, "prev": 0xb6, "stop": 0xb7,
}

// HID keyboard-page codes, named as in ch57x-keyboard-tool.
var padCodes = func() map[string]byte {
	m := map[string]byte{}
	for i, c := range "abcdefghijklmnopqrstuvwxyz" {
		m[string(c)] = 0x04 + byte(i)
	}
	for i, c := range "1234567890" {
		m[string(c)] = 0x1e + byte(i)
	}
	names := []string{"enter", "escape", "backspace", "tab", "space", "minus", "equal", "leftbracket",
		"rightbracket", "backslash", "nonushash", "semicolon", "quote", "grave", "comma", "dot", "slash", "capslock"}
	for i, n := range names {
		m[n] = 0x28 + byte(i)
	}
	m["esc"] = m["escape"]
	for i := 0; i < 12; i++ {
		m[fmt.Sprintf("f%d", i+1)] = 0x3a + byte(i)
	}
	for i, n := range []string{"printscreen", "scrolllock", "pause", "insert", "home", "pageup", "delete",
		"end", "pagedown", "right", "left", "down", "up"} {
		m[n] = 0x46 + byte(i)
	}
	for i := 0; i < 12; i++ {
		m[fmt.Sprintf("f%d", i+13)] = 0x68 + byte(i)
	}
	return m
}()

type padChord struct{ mods, code byte }

// padAction is one parsed action: either media, or a sequence of chords.
type padAction struct {
	media  uint16
	chords []padChord
}

// parsePadAction checks an action with the limits of the built-in pad.
func parsePadAction(s string) (padAction, error) { return parsePadActionLimits(s, 5, true) }

func parsePadActionLimits(s string, maxSeq int, modsFirstOnly bool) (padAction, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return padAction{chords: []padChord{{}}}, nil // a key that sends nothing
	}
	if m, ok := padMedia[s]; ok {
		return padAction{media: m}, nil
	}
	var a padAction
	for i, part := range strings.Split(s, ",") {
		var c padChord
		bits := strings.Split(strings.TrimSpace(part), "-")
		for j, b := range bits {
			if m, ok := padModifiers[b]; ok {
				c.mods |= m
				continue
			}
			code, ok := padCodes[b]
			if !ok {
				return a, fmt.Errorf("unknown key %q in %q", b, s)
			}
			if j != len(bits)-1 {
				return a, fmt.Errorf("%q: the key has to come last, after ctrl/alt/shift/win", part)
			}
			c.code = code
		}
		if c.mods == 0 && c.code == 0 {
			return a, fmt.Errorf("empty key in %q", s)
		}
		if i > 0 && c.mods != 0 && modsFirstOnly {
			// The 3-key pads only apply modifiers to the first key of a sequence.
			return a, fmt.Errorf("%q: on this pad only the first key of a sequence can use ctrl/alt/shift/win", s)
		}
		a.chords = append(a.chords, c)
	}
	if len(a.chords) > maxSeq {
		return a, fmt.Errorf("%q: at most %d keys in a sequence on this pad", s, maxSeq)
	}
	return a, nil
}

func padMsg(out *[][]byte, b ...byte) {
	m := make([]byte, 64)
	copy(m, b)
	*out = append(*out, m)
}

// padBind encodes one control's binding on layer 0 (the 8890 protocol).
func padBind(out *[][]byte, keyID byte, a padAction) {
	const layer = 1
	padMsg(out, 0x03, 0xfe, layer, 0x01, 0x01)
	if a.media != 0 {
		padMsg(out, 0x03, keyID, layer<<4|0x02, byte(a.media), byte(a.media>>8))
	} else {
		n := byte(len(a.chords))
		padMsg(out, 0x03, keyID, layer<<4|0x01, n, 0, 0, 0) // the pad expects an empty entry first
		for i, c := range a.chords {
			padMsg(out, 0x03, keyID, layer<<4|0x01, n, byte(i+1), c.mods, c.code)
		}
	}
	padMsg(out, 0x03, 0xaa, 0xaa)
}

// padLightMessages sets the LED effect (k8890.rs set_led). On the 3-key test pad,
// modes 1 and 2 are light effects; 0, 3 and 4-8 show nothing (tested 2026-11-09).
func padLightMessages(mode int) [][]byte {
	if mode < 0 || mode > 2 {
		mode = 0
	}
	var out [][]byte
	padMsg(&out, 0x03, 0xa1, 0x01)
	padMsg(&out, 0x03, 0xb0, 0x18, byte(mode))
	padMsg(&out, 0x03, 0xaa, 0xa1)
	return out
}
