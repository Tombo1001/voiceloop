package main

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestFixWavKokoro(t *testing.T) {
	raw, err := os.ReadFile(os.Getenv("KOKORO_WAV"))
	if err != nil {
		t.Skip("set KOKORO_WAV")
	}
	out, ms, err := fixWav(raw, 400)
	if err != nil {
		t.Fatal(err)
	}
	dataLen := binary.LittleEndian.Uint32(out[40:44])
	if int(dataLen) != len(out)-44 || binary.LittleEndian.Uint32(out[4:8]) != uint32(len(out)-8) {
		t.Fatalf("sizes wrong: data %d, file %d", dataLen, len(out))
	}
	t.Logf("in %d bytes, out %d bytes, %d ms incl. 400 ms pre-roll", len(raw), len(out), ms)
	os.WriteFile(os.Getenv("KOKORO_WAV")+".fixed.wav", out, 0o644)
}

func TestSplitChunks(t *testing.T) {
	text := "Short one. This is the second sentence, a bit longer than the first. Third sentence here! And a fourth: with a colon. Finally the fifth sentence ends it all."
	got := splitChunks(text, 40, 90)
	joined := ""
	for i, c := range got {
		t.Logf("%d (%d): %q", i, len(c), c)
		if c == "" {
			t.Fatal("empty chunk")
		}
		joined += c + " "
	}
	if trim(joined) != text {
		t.Fatalf("chunks lose text:\n%q\n%q", trim(joined), text)
	}
	if len(splitChunks("no full stop at all", 10, 20)) != 1 {
		t.Fatal("unterminated text should be one chunk")
	}
}

func TestScaleVolume(t *testing.T) {
	pcm := []byte{0x00, 0x40, 0x00, 0xC0} // +16384, -16384
	half := scaleVolume(pcm, 50)
	if v := int16(uint16(half[0]) | uint16(half[1])<<8); v != 4096 {
		t.Fatalf("50%% of 16384 should be 4096 on the squared curve, got %d", v)
	}
	if v := int16(uint16(half[2]) | uint16(half[3])<<8); v != -4096 {
		t.Fatalf("negative sample wrong: %d", v)
	}
	if z := scaleVolume(pcm, 0); z[1] != 0 || z[3] != 0 {
		t.Fatal("0% should be silent")
	}
}

func TestLimitText(t *testing.T) {
	text := "First sentence here. Second one is a bit longer than that. Two things worth knowing: Third. Fourth sentence."
	if got := limitText(text, 0); got != text {
		t.Fatal("0 should mean no limit")
	}
	if got := limitText(text, 60); got != "First sentence here. Second one is a bit longer than that." {
		t.Fatalf("limit 60: %q", got)
	}
	// Would end on the lead-in, so it's dropped.
	if got := limitText(text, 85); got != "First sentence here. Second one is a bit longer than that." {
		t.Fatalf("limit 85 should drop the trailing lead-in: %q", got)
	}
	if got := limitText(text, 5000); got != text {
		t.Fatal("big limit should keep everything")
	}
}
