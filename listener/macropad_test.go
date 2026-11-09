package main

import (
	"bytes"
	"testing"
)

func padTrim(b []byte) []byte { return bytes.TrimRight(b, "\x00") }

// The expected bytes are the ones in ch57x-keyboard-tool's own k8890.rs tests.
func TestPadBindMatchesUpstream(t *testing.T) {
	var out [][]byte
	a, _ := parsePadAction("ctrl-a")
	padBind(&out, 1, a)
	want := [][]byte{
		{0x03, 0xfe, 0x01, 0x01, 0x01},
		{0x03, 0x01, 0x11, 0x01},
		{0x03, 0x01, 0x11, 0x01, 0x01, 0x01, 0x04},
		{0x03, 0xaa, 0xaa},
	}
	check(t, "ctrl-a", out, want)

	out = nil
	a, _ = parsePadAction("volumeup")
	padBind(&out, 2, a)
	check(t, "volumeup", out, [][]byte{{0x03, 0xfe, 0x01, 0x01, 0x01}, {0x03, 0x02, 0x12, 0xe9, 0x00}, {0x03, 0xaa, 0xaa}})
}

func check(t *testing.T, name string, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d messages, want %d", name, len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != 64 {
			t.Fatalf("%s: message %d is %d bytes", name, i, len(got[i]))
		}
		if !bytes.Equal(padTrim(got[i]), padTrim(want[i])) {
			t.Fatalf("%s: message %d = % x, want % x", name, i, padTrim(got[i]), want[i])
		}
	}
}

func TestPadParse(t *testing.T) {
	for _, s := range []string{"ctrl-space", "enter", "ctrl-alt-x", "f13", "volumedown", "", "ctrl-c,v"} {
		if _, err := parsePadAction(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"ctrl-nope", "x-ctrl", "a,ctrl-b", "a,b,c,d,e,f"} {
		if _, err := parsePadAction(s); err == nil {
			t.Errorf("%q should fail", s)
		}
	}
	a, _ := parsePadAction("ctrl-alt-x")
	if a.chords[0] != (padChord{0x05, 0x1b}) {
		t.Errorf("ctrl-alt-x = %+v", a.chords[0])
	}
	a, _ = parsePadAction("enter")
	if a.chords[0].code != 0x28 {
		t.Errorf("enter = %x", a.chords[0].code)
	}
	a, _ = parsePadAction("f13")
	if a.chords[0].code != 0x68 {
		t.Errorf("f13 = %x", a.chords[0].code)
	}
}
