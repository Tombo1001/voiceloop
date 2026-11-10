package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// tinyWav is a valid 16-bit mono WAV with a few samples.
func tinyWav() []byte {
	return wavBytes(1, 1, 24000, 16, []byte{0, 0, 0x10, 0, 0xf0, 0xff, 0, 0})
}

func wavBytes(tag, channels, rate, bits int, data []byte) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, uint16(tag))
	b = binary.LittleEndian.AppendUint16(b, uint16(channels))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate*channels*bits/8))
	b = binary.LittleEndian.AppendUint16(b, uint16(channels*bits/8))
	b = binary.LittleEndian.AppendUint16(b, uint16(bits))
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	return append(b, data...)
}

// fakeServer answers like a speech server. paths maps a path to a status and body; the speech
// endpoint records the request body and returns a WAV. key, if set, is required as a bearer token.
func fakeServer(t *testing.T, key string, paths map[string]string) (*httptest.Server, *map[string]any) {
	var last map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key != "" && r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/v1/audio/speech" && r.Method == "POST" {
			b, _ := io.ReadAll(r.Body)
			last = map[string]any{}
			json.Unmarshal(b, &last)
			w.Write(tinyWav())
			return
		}
		body, ok := paths[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

func TestKokoroStyleServer(t *testing.T) {
	srv, last := fakeServer(t, "", map[string]string{
		"/health":          `{"status":"healthy"}`,
		"/v1/audio/voices": `{"voices":["bf_emma","am_adam","bf_emma"]}`,
	})
	s := ttsServer{URL: srv.URL, Model: "kokoro"}
	r := testSpeechServer(s, "bf_emma")
	if !r.OK || strings.Join(r.Voices, ",") != "bf_emma,am_adam" {
		t.Fatalf("test: %+v", r)
	}
	if _, err := synthWith(s, "bf_emma", 1.1, "Hello."); err != nil {
		t.Fatal(err)
	}
	if (*last)["model"] != "kokoro" || (*last)["stream"] != false || (*last)["response_format"] != "wav" || (*last)["speed"] != 1.1 {
		t.Fatalf("request body: %v", *last)
	}
}

func TestOpenAIStyleServerWithKey(t *testing.T) {
	srv, last := fakeServer(t, "sekrit", map[string]string{
		"/v1/models":       `{"data":[{"id":"speaches-ai/Kokoro-82M-v1.0-ONNX"}]}`,
		"/v1/audio/voices": `{"data":[{"model":"tts","voices":["af_heart",{"voice_id":"bf_alice"}]},{"model":"other"}]}`,
	})
	if r := testSpeechServer(ttsServer{URL: srv.URL}, "af_heart"); r.OK || !strings.Contains(r.Error, "API key") {
		t.Fatalf("without the key: %+v", r)
	}
	s := ttsServer{URL: srv.URL, Model: "tts-1", Key: "sekrit"}
	r := testSpeechServer(s, "af_heart")
	if !r.OK || strings.Join(r.Voices, ",") != "af_heart,bf_alice" {
		t.Fatalf("with the key: %+v", r)
	}
	if _, err := synthWith(s, "af_heart", 1, "Hello."); err != nil {
		t.Fatal(err)
	}
	if (*last)["model"] != "tts-1" {
		t.Fatalf("model: %v", *last)
	}
	if _, has := (*last)["stream"]; has {
		t.Fatalf("non-Kokoro servers shouldn't get stream: %v", *last)
	}
}

func TestServerWithOnlySpeech(t *testing.T) {
	srv, _ := fakeServer(t, "", nil)
	if r := testSpeechServer(ttsServer{URL: srv.URL}, "alloy"); !r.OK || len(r.Voices) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestNotASpeechServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/audio/speech" {
			w.Write([]byte("<html>hello</html>"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	r := testSpeechServer(ttsServer{URL: srv.URL}, "x")
	if r.OK || !strings.Contains(r.Error, "OpenAI-style") {
		t.Fatalf("%+v", r)
	}
	if _, err := synthWith(ttsServer{URL: srv.URL}, "x", 1, "hi"); err == nil || !strings.Contains(err.Error(), "other than WAV") {
		t.Fatalf("synth: %v", err)
	}
	if r := testSpeechServer(ttsServer{URL: "http://127.0.0.1:1"}, "x"); r.OK || !strings.HasPrefix(r.Error, "Can't reach") {
		t.Fatalf("closed port: %+v", r)
	}
}

func TestParseVoices(t *testing.T) {
	for in, want := range map[string]string{
		`{"voices":["a","b"]}`:                 "a,b",
		`["a",{"id":"b"},{"name":"c"}]`:        "a,b,c",
		`{"data":[{"voice_id":"a","id":"x"}]}`: "a",
		`{"data":[{"model":"m","voices":["a","b"]},{"model":"n","voices":[{"name":"c"}]}]}`:          "a,b,c",
		`{"voices":[{"name":"Heart","language":"en","gender":"f","id":"af_heart"}],"object":"list"}`: "af_heart",
		`{"status":"ok","voices":["Abigail.wav","Emily.wav"]}`:                                       "Abigail.wav,Emily.wav",
		`{"other":1}`: "",
		`nonsense`:    "",
	} {
		if got := strings.Join(parseVoices([]byte(in)), ","); got != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}

func TestToMono16(t *testing.T) {
	samples := func(wav []byte) []int16 {
		fixed, _, err := fixWav(wav, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ch, bits := binary.LittleEndian.Uint16(fixed[22:24]), binary.LittleEndian.Uint16(fixed[34:36]); ch != 1 || bits != 16 {
			t.Fatalf("header says %d channels, %d bits", ch, bits)
		}
		var out []int16
		for i := 44; i+1 < len(fixed); i += 2 {
			out = append(out, int16(binary.LittleEndian.Uint16(fixed[i:])))
		}
		return out
	}
	near := func(got []int16, want ...int16) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range got {
			if d := int(got[i]) - int(want[i]); d < -2 || d > 2 {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	}
	le16 := func(v ...int16) (b []byte) {
		for _, x := range v {
			b = binary.LittleEndian.AppendUint16(b, uint16(x))
		}
		return
	}
	// Stereo 16-bit averages the channels.
	near(samples(wavBytes(1, 2, 22050, 16, le16(1000, 3000, -2000, 0))), 2000, -1000)
	// 32-bit float, mono.
	f32 := func(v ...float32) (b []byte) {
		for _, x := range v {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(x))
		}
		return
	}
	near(samples(wavBytes(3, 1, 44100, 32, f32(0.5, -0.25, 2))), 16384, -8192, 32767)
	// 24-bit PCM.
	near(samples(wavBytes(1, 1, 48000, 24, []byte{0, 0, 0x40, 0, 0, 0xc0})), 16384, -16384)
	// 8-bit unsigned PCM.
	near(samples(wavBytes(1, 1, 8000, 8, []byte{128, 192, 64})), 0, 16384, -16384)
	// WAVE_FORMAT_EXTENSIBLE wrapping float.
	ext := wavBytes(3, 1, 24000, 32, f32(0.5))
	ext = append(ext[:16], append(binary.LittleEndian.AppendUint32(nil, 40), ext[20:]...)...)
	ext[20], ext[21] = 0xFE, 0xFF                                    // tag: extensible
	ext = append(ext[:36], append(make([]byte, 24), ext[36:]...)...) // cbSize, valid bits, mask, GUID
	binary.LittleEndian.PutUint16(ext[36+8:], 3)                     // sub-format starts with the real tag
	near(samples(ext), 16384)
	// Unsupported: 12-bit.
	if _, _, err := fixWav(wavBytes(1, 1, 8000, 12, []byte{1, 2, 3}), 0); err == nil {
		t.Fatal("12-bit should be refused")
	}
}

// With SPEECH_SERVER_URL set (and SPEECH_SERVER_MODEL, SPEECH_SERVER_VOICE, optionally
// SPEECH_SERVER_KEY), test a real server end to end.
func TestRealSpeechServer(t *testing.T) {
	url := os.Getenv("SPEECH_SERVER_URL")
	if url == "" {
		t.Skip("set SPEECH_SERVER_URL to test a real speech server")
	}
	s := ttsServer{URL: url, Model: os.Getenv("SPEECH_SERVER_MODEL"), Key: os.Getenv("SPEECH_SERVER_KEY")}
	voice := os.Getenv("SPEECH_SERVER_VOICE")
	r := testSpeechServer(s, voice)
	t.Logf("connection test: ok=%v %d ms, %d voices %v, error %q", r.OK, r.Ms, len(r.Voices), head(r.Voices, 5), r.Error)
	if !r.OK {
		t.Fatal(r.Error)
	}
	raw, err := synthWith(s, voice, 1, "Hello from voiceloop. This is a test of another speech server.")
	if err != nil {
		t.Fatal(err)
	}
	fixed, ms, err := fixWav(raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("got %d bytes; header %d Hz, %d channels, %d bits; %d ms of audio after conversion",
		len(raw), binary.LittleEndian.Uint32(raw[24:28]), binary.LittleEndian.Uint16(raw[22:24]), binary.LittleEndian.Uint16(raw[34:36]), ms)
	if ms < 1000 {
		t.Fatalf("only %d ms of audio", ms)
	}
	if out := os.Getenv("SPEECH_SERVER_SAVE"); out != "" {
		os.WriteFile(out, fixed, 0o644)
	}
}

func head(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
