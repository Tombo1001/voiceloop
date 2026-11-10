package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// fixWav finds the PCM data, rebuilds a clean 44-byte header with true sizes
// (Kokoro streams a header with placeholder lengths), converts it to 16-bit mono, and prepends silence.
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
	mono, rate, err := toMono16(fmtChunk, data)
	if err != nil {
		return nil, 0, err
	}
	byteRate := rate * 2
	silence := make([]byte, (byteRate*prerollMs/1000)&^1)
	pcm := append(silence, mono...)
	var out bytes.Buffer
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, uint32(36+len(pcm)))
	out.WriteString("WAVEfmt ")
	binary.Write(&out, binary.LittleEndian, uint32(16))
	binary.Write(&out, binary.LittleEndian, [4]uint32{1 | 1<<16, uint32(rate), uint32(byteRate), 2 | 16<<16}) // PCM, mono, rate, byte rate, align 2, 16 bits
	out.WriteString("data")
	binary.Write(&out, binary.LittleEndian, uint32(len(pcm)))
	out.Write(pcm)
	ms := len(pcm) * 1000 / byteRate
	return out.Bytes(), ms, nil
}

// toMono16 converts PCM (8, 16, 24 or 32-bit) or float (32 or 64-bit) audio with any number of
// channels to 16-bit mono, which is what playback and the pill's level meter expect. Kokoro
// already sends 16-bit mono; other speech servers may not.
func toMono16(fmtChunk, data []byte) ([]byte, int, error) {
	tag := binary.LittleEndian.Uint16(fmtChunk[0:2])
	channels := int(binary.LittleEndian.Uint16(fmtChunk[2:4]))
	rate := int(binary.LittleEndian.Uint32(fmtChunk[4:8]))
	bits := int(binary.LittleEndian.Uint16(fmtChunk[14:16]))
	if tag == 0xFFFE && len(fmtChunk) >= 26 { // WAVE_FORMAT_EXTENSIBLE: the real tag starts the sub-format GUID
		tag = binary.LittleEndian.Uint16(fmtChunk[24:26])
	}
	if channels < 1 || rate < 1000 || rate > 384000 {
		return nil, 0, fmt.Errorf("bad wav format")
	}
	bytesPer := bits / 8
	var sample func(b []byte) float64
	switch {
	case tag == 1 && bits == 16:
		sample = func(b []byte) float64 { return float64(int16(binary.LittleEndian.Uint16(b))) / 32768 }
	case tag == 1 && bits == 8:
		sample = func(b []byte) float64 { return (float64(b[0]) - 128) / 128 }
	case tag == 1 && bits == 24:
		sample = func(b []byte) float64 {
			return float64(int32(uint32(b[0])<<8|uint32(b[1])<<16|uint32(b[2])<<24)>>8) / 8388608
		}
	case tag == 1 && bits == 32:
		sample = func(b []byte) float64 { return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648 }
	case tag == 3 && bits == 32:
		sample = func(b []byte) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(b))) }
	case tag == 3 && bits == 64:
		sample = func(b []byte) float64 { return math.Float64frombits(binary.LittleEndian.Uint64(b)) }
	default:
		return nil, 0, fmt.Errorf("unsupported wav format (tag %d, %d bits)", tag, bits)
	}
	frame := bytesPer * channels
	if tag == 1 && bits == 16 && channels == 1 {
		return append([]byte(nil), data[:len(data)&^1]...), rate, nil
	}
	n := len(data) / frame
	out := make([]byte, n*2)
	for i := 0; i < n; i++ {
		sum := 0.0
		for c := 0; c < channels; c++ {
			sum += sample(data[i*frame+c*bytesPer:])
		}
		v := math.Round(sum / float64(channels) * 32767)
		v = math.Max(-32768, math.Min(32767, v))
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(v)))
	}
	return out, rate, nil
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
