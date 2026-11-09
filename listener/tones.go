package main

// Chimes, synthesised in code so there are no sound files to ship.

import (
	"bytes"
	"encoding/binary"
	"math"
)

const toneRate = 24000

type note struct {
	freq  float64 // Hz
	start float64 // seconds from the start of the chime
	dur   float64 // seconds
	decay float64 // higher = shorter ring
}

// Soft bell: fundamental plus a little 2nd and 3rd harmonic, quick attack, exponential decay.
func renderNotes(notes []note, gain float64) []byte {
	total := 0.0
	for _, n := range notes {
		total = math.Max(total, n.start+n.dur)
	}
	samples := make([]float64, int(total*toneRate)+1)
	for _, n := range notes {
		s0 := int(n.start * toneRate)
		for i := 0; i < int(n.dur*toneRate) && s0+i < len(samples); i++ {
			t := float64(i) / toneRate
			env := math.Min(1, t/0.004) * math.Exp(-t*n.decay)
			ph := 2 * math.Pi * n.freq * t
			samples[s0+i] += env * (math.Sin(ph) + 0.35*math.Sin(2*ph) + 0.12*math.Sin(3*ph))
		}
	}
	var pcm bytes.Buffer
	for _, v := range samples {
		binary.Write(&pcm, binary.LittleEndian, int16(math.Max(-1, math.Min(1, v*gain))*32767))
	}
	return pcm.Bytes()
}

// chimePCM returns 16-bit mono PCM at toneRate for each kind of event.
func chimePCM(kind string) []byte {
	switch kind {
	case "question": // three rising notes, friendly: E5 G5 C6
		return renderNotes([]note{
			{659.3, 0.00, 0.45, 7}, {784.0, 0.12, 0.45, 7}, {1046.5, 0.24, 0.7, 5},
		}, 0.22)
	case "approval": // insistent ding-dong-ding: A5 E5 A5, twice
		return renderNotes([]note{
			{880.0, 0.00, 0.30, 9}, {659.3, 0.16, 0.30, 9}, {880.0, 0.32, 0.30, 9},
			{880.0, 0.62, 0.30, 9}, {659.3, 0.78, 0.30, 9}, {880.0, 0.94, 0.5, 6},
		}, 0.24)
	case "on": // voice mode on: two rising notes
		return renderNotes([]note{{659.3, 0, 0.25, 12}, {880.0, 0.1, 0.3, 10}}, 0.2)
	case "off": // voice mode off: two falling notes
		return renderNotes([]note{{880.0, 0, 0.25, 12}, {659.3, 0.1, 0.3, 10}}, 0.2)
	}
	return nil
}

// wavFromPCM wraps 16-bit mono PCM in a WAV header.
func wavFromPCM(pcm []byte, rate int) []byte {
	var w bytes.Buffer
	w.WriteString("RIFF")
	binary.Write(&w, binary.LittleEndian, uint32(36+len(pcm)))
	w.WriteString("WAVEfmt ")
	binary.Write(&w, binary.LittleEndian, uint32(16))
	binary.Write(&w, binary.LittleEndian, []uint16{1, 1})
	binary.Write(&w, binary.LittleEndian, []uint32{uint32(rate), uint32(rate * 2)})
	binary.Write(&w, binary.LittleEndian, []uint16{2, 16})
	w.WriteString("data")
	binary.Write(&w, binary.LittleEndian, uint32(len(pcm)))
	w.Write(pcm)
	return w.Bytes()
}
