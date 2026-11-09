package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// fixWav finds the PCM data, rebuilds a clean 44-byte header with true sizes
// (Kokoro streams a header with placeholder lengths), and prepends silence.
func fixWav(b []byte, prerollMs int) ([]byte, int, error) {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("not a wav")
	}
	var fmtChunk []byte
	var data []byte
	for i := 12; i+8 <= len(b); {
		id := string(b[i : i+4])
		sz := int(binary.LittleEndian.Uint32(b[i+4 : i+8]))
		body := i + 8
		if id == "data" {
			end := body + sz
			if sz == 0 || sz == 0xFFFFFFFF || end > len(b) {
				end = len(b)
			}
			data = b[body:end]
			break
		}
		if body+sz > len(b) {
			break
		}
		if id == "fmt " {
			fmtChunk = b[body : body+sz]
		}
		i = body + sz + sz%2
	}
	if fmtChunk == nil || data == nil || len(fmtChunk) < 16 {
		return nil, 0, fmt.Errorf("wav chunks missing")
	}
	byteRate := int(binary.LittleEndian.Uint32(fmtChunk[8:12]))
	align := int(binary.LittleEndian.Uint16(fmtChunk[12:14]))
	if byteRate == 0 || align == 0 {
		return nil, 0, fmt.Errorf("bad wav format")
	}
	silence := make([]byte, (byteRate*prerollMs/1000)/align*align)
	pcm := append(silence, data[:len(data)/align*align]...)
	var out bytes.Buffer
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, uint32(36+len(pcm)))
	out.WriteString("WAVEfmt ")
	binary.Write(&out, binary.LittleEndian, uint32(16))
	out.Write(fmtChunk[:16])
	out.WriteString("data")
	binary.Write(&out, binary.LittleEndian, uint32(len(pcm)))
	out.Write(pcm)
	ms := len(pcm) * 1000 / byteRate
	return out.Bytes(), ms, nil
}

// splitChunks breaks text at sentence ends into pieces of about max characters.
// The first piece is kept shorter so speech starts sooner.
func splitChunks(text string, first, max int) []string {
	var sentences []string
	start := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		if (c == '.' || c == '!' || c == '?' || c == ':') && (i+1 == len(text) || text[i+1] == ' ') {
			sentences = append(sentences, text[start:i+1])
			start = i + 1
		}
	}
	if start < len(text) {
		sentences = append(sentences, text[start:])
	}
	var out []string
	cur := ""
	limit := first
	for _, s := range sentences {
		if cur != "" && len(cur)+len(s) > limit {
			out = append(out, trim(cur))
			cur = ""
			limit = max
		}
		cur += s
	}
	if trim(cur) != "" {
		out = append(out, trim(cur))
	}
	return out
}

func trim(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

// scaleVolume returns a copy of 16-bit PCM scaled by volume percent (0..100).
func scaleVolume(pcm []byte, volume int) []byte {
	out := make([]byte, len(pcm)&^1)
	g := float64(volume) / 100
	g = g * g // perceptual-ish curve: 50% sounds about half as loud
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8))
		s := int16(v * g)
		out[i], out[i+1] = byte(s), byte(uint16(s)>>8)
	}
	return out
}

// limitText keeps whole sentences up to max characters (0 = no limit) and never ends
// on a lead-in line such as "Two things worth knowing:".
func limitText(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	sentences := splitChunks(text, 1, 1) // one sentence per chunk
	var picked []string
	n := 0
	for _, s := range sentences {
		if len(picked) > 0 && n+1+len(s) > max {
			break
		}
		picked = append(picked, s)
		n += len(s) + 1
	}
	for len(picked) > 1 && strings.HasSuffix(picked[len(picked)-1], ":") {
		picked = picked[:len(picked)-1]
	}
	out := strings.Join(picked, " ")
	if len(out) > max*2 { // a single enormous "sentence"
		out = out[:max]
	}
	return out
}
