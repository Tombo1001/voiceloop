package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultProfile(t *testing.T) {
	p, err := ParsePadProfile(defaultPadXML)
	if err != nil {
		t.Fatal(err)
	}
	if p.Action("key2") != "ctrl-space,slash" || p.Action("knob1.right") != "volumeup" || len(p.Lights) != 3 {
		t.Fatalf("default profile: %+v", p.Mapping)
	}
	msgs, err := padMessagesFor(p)
	if err != nil {
		t.Fatal(err)
	}
	// zero report + key1/key3/knob press (one chord: 4 msgs each) + key2 (two chords: 5)
	// + two media turns (3 each) + lights (3)
	if want := 1 + 3*4 + 5 + 2*3 + 3; len(msgs) != want {
		t.Fatalf("%d messages, want %d", len(msgs), want)
	}
}

func TestProfileRoundTrip(t *testing.T) {
	p, _ := ParsePadProfile(defaultPadXML)
	if err := p.SetAction("key1", "f13"); err != nil {
		t.Fatal(err)
	}
	out := p.Marshal()
	q, err := ParsePadProfile(out)
	if err != nil {
		t.Fatalf("re-reading an export failed: %v\n%s", err, out)
	}
	if q.Action("key1") != "f13" || q.Name != p.Name || q.USB != p.USB || len(q.Mapping.Bind) != 6 {
		t.Fatalf("round trip lost something:\n%s", out)
	}
	if !bytes.Contains(out, []byte(`xmlns="`+padNS+`"`)) {
		t.Fatal("export has no namespace")
	}
}

func TestKnobLeftFlipsCodes(t *testing.T) {
	p, _ := ParsePadProfile(defaultPadXML)
	if c := p.physicalCodes(); c["key1"] != 1 || c["key3"] != 3 || c["knob1.press"] != 14 {
		t.Fatalf("normal: %v", c)
	}
	p.Mapping.KnobSide = "left"
	if c := p.physicalCodes(); c["key1"] != 3 || c["key2"] != 2 || c["key3"] != 1 || c["knob1.press"] != 14 {
		t.Fatalf("turned round: %v", c)
	}
}

func TestBadProfiles(t *testing.T) {
	good := string(defaultPadXML)
	for name, c := range map[string]struct{ from, to, want string }{
		"protocol":  {"<protocol>ch57x-8890</protocol>", "<protocol>razer</protocol>", "doesn't know the protocol"},
		"grid":      {`rows="1" columns="3"`, `rows="2" columns="3"`, "means 6 keys"},
		"dup code":  {`<key id="key2" code="2"/>`, `<key id="key2" code="1"/>`, "both use code 1"},
		"bad bind":  {`control="key3"`, `control="key9"`, "no such key"},
		"bad act":   {`action="enter"`, `action="enterr"`, `unknown key "enterr"`},
		"lights":    {`lights="0">`, `lights="7">`, "isn't one of the listed light modes"},
		"hex":       {`vendorId="0x1189"`, `vendorId="banana"`, "should be a hex number"},
		"version":   {`version="1"`, `version="2"`, "version 2 isn't supported"},
		"mods":      {`action="ctrl-space,slash"`, `action="a,ctrl-b"`, "only the first key"},
		"knob bare": {`control="knob1.press"`, `control="knob1"`, "no such key"},
	} {
		if !strings.Contains(good, c.from) {
			t.Fatalf("%s: test text %q not in the default profile", name, c.from)
		}
		_, err := ParsePadProfile([]byte(strings.Replace(good, c.from, c.to, 1)))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
	if _, err := ParsePadProfile([]byte("<html/>")); err == nil {
		t.Error("html accepted as a profile")
	}
}

func TestPadStoreMigrateAndBackup(t *testing.T) {
	dir := t.TempDir()
	var s padStore
	legacy := &PadMap{Keys: [3]string{"ctrl-space", "ctrl-alt-x", "enter"}, KnobCCW: "volumedown", KnobPress: "ctrl-alt-x", KnobCW: "volumeup", Lights: 2}
	migrated, err := s.load(dir, legacy)
	if err != nil || !migrated {
		t.Fatalf("migrate: %v %v", migrated, err)
	}
	if p := s.get(); p.Action("key2") != "ctrl-alt-x" || p.Mapping.Lights != 2 {
		t.Fatalf("legacy map not carried over: %+v", p.Mapping)
	}
	// A second start reads the file and ignores the legacy map.
	var s2 padStore
	if m, err := s2.load(dir, legacy); m || err != nil || s2.get().Action("key2") != "ctrl-alt-x" {
		t.Fatalf("reload: %v %v", m, err)
	}
	// Import: the old file is copied to pad-profiles\ first.
	p, _ := ParsePadProfile(defaultPadXML)
	name, err := s2.replace(p, "import")
	if err != nil || !strings.Contains(name, "before-import") {
		t.Fatalf("replace: %q %v", name, err)
	}
	old, err := os.ReadFile(filepath.Join(dir, "pad-profiles", name))
	if err != nil || !bytes.Contains(old, []byte(`action="ctrl-alt-x"`)) {
		t.Fatalf("backup missing or wrong: %v", err)
	}
	if s2.get().Action("key2") != "ctrl-space,slash" {
		t.Fatal("import didn't replace the profile")
	}
}

func TestProfileOldNamespaceStillLoads(t *testing.T) {
	old := bytes.Replace(defaultPadXML, []byte(padNS), []byte(padNSOld), 1)
	if bytes.Equal(old, defaultPadXML) {
		t.Fatal("built-in profile doesn't use padNS")
	}
	if _, err := ParsePadProfile(old); err != nil {
		t.Fatalf("a profile in the old namespace should load: %v", err)
	}
	other := bytes.Replace(defaultPadXML, []byte(padNS), []byte("https://example.com/other"), 1)
	if _, err := ParsePadProfile(other); err == nil {
		t.Fatal("a profile in an unknown namespace should be refused")
	}
}
