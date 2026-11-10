package main

// Speech servers. voiceloop sends text to an OpenAI-style POST /v1/audio/speech endpoint and
// plays the WAV that comes back. Kokoro-FastAPI is the default, but any server with that
// endpoint works: the model name and an optional API key are settings.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ttsServer is where speech comes from.
type ttsServer struct {
	URL   string // e.g. http://127.0.0.1:8880, no trailing slash
	Model string // "kokoro" for Kokoro-FastAPI
	Key   string // sent as a bearer token when set
}

func (s ttsServer) request(c *http.Client, method, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(method, s.URL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.Key != "" {
		req.Header.Set("Authorization", "Bearer "+s.Key)
	}
	return c.Do(req)
}

// speechBody is the JSON for /v1/audio/speech. Kokoro-FastAPI streams by default, so it gets
// "stream": false. Other servers don't get the field, in case they reject unknown ones.
func speechBody(model, voice string, speed float64, text string) []byte {
	if model == "" {
		model = "kokoro"
	}
	body := map[string]any{"model": model, "voice": voice, "input": text, "response_format": "wav", "speed": speed}
	if model == "kokoro" {
		body["stream"] = false
	}
	b, _ := json.Marshal(body)
	return b
}

func synthWith(srv ttsServer, voice string, speed float64, text string) ([]byte, error) {
	if srv.URL == "" {
		return nil, fmt.Errorf("no speech server set")
	}
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := srv.request(c, "POST", "/v1/audio/speech", speechBody(srv.Model, voice, speed, text))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("speech server %d: %s", resp.StatusCode, clip(b, 300))
	}
	if len(b) < 12 || string(b[0:4]) != "RIFF" {
		return nil, fmt.Errorf("speech server sent something other than WAV audio (%d bytes)", len(b))
	}
	return b, nil
}

type speechTestResult struct {
	OK     bool     `json:"ok"`
	Ms     int64    `json:"ms"`
	Error  string   `json:"error,omitempty"`
	Voices []string `json:"voices,omitempty"`
}

// testSpeechServer checks the server answers, then asks for its voices. Kokoro-FastAPI and
// Speaches have /health, LocalAI has /v1/models, Chatterbox and Orpheus have /v1/audio/voices.
// Servers with none of them are tried with a one-word speech request in the given voice.
func testSpeechServer(srv ttsServer, voice string) speechTestResult {
	if srv.URL == "" {
		return speechTestResult{Error: "Enter the server address first"}
	}
	c := &http.Client{Timeout: 8 * time.Second}
	start := time.Now()
	ok, status := false, ""
	for _, path := range []string{"/health", "/v1/models", "/v1/audio/voices"} {
		resp, err := srv.request(c, "GET", path, nil)
		if err != nil {
			return speechTestResult{Error: "Can't reach the server: " + shortErr(err)}
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			ok = true
			break
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return speechTestResult{Ms: time.Since(start).Milliseconds(), Error: "The server wants an API key (it answered " + resp.Status + ")"}
		}
		status = resp.Status
	}
	if !ok {
		if _, err := synthWith(srv, voice, 1, "Test."); err != nil {
			return speechTestResult{Ms: time.Since(start).Milliseconds(),
				Error: "No speech from the server (" + status + " for the usual checks, then " + err.Error() + "). Is it an OpenAI-style speech server?"}
		}
	}
	res := speechTestResult{OK: true, Ms: time.Since(start).Milliseconds()}
	if resp, err := srv.request(c, "GET", "/v1/audio/voices", nil); err == nil {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode == 200 {
			res.Voices = parseVoices(b)
		}
	}
	return res
}

// parseVoices reads a voice list in the shapes servers use: {"voices": [...]} (Kokoro-FastAPI,
// Speaches, Chatterbox, Orpheus), {"data": [{"model": ..., "voices": [...]}]} (LocalAI) or a bare
// array, holding strings or objects with an id, voice_id or name.
func parseVoices(b []byte) []string {
	var items []json.RawMessage
	var wrapped struct {
		Voices []json.RawMessage `json:"voices"`
		Data   []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(b, &items) != nil {
		if json.Unmarshal(b, &wrapped) != nil {
			return nil
		}
		items = append(wrapped.Voices, wrapped.Data...)
	}
	var out []string
	seen := map[string]bool{}
	for _, raw := range items {
		var s string
		var o struct {
			ID      string            `json:"id"`
			VoiceID string            `json:"voice_id"`
			Name    string            `json:"name"`
			Voices  []json.RawMessage `json:"voices"`
		}
		var found []string
		if json.Unmarshal(raw, &s) == nil {
			found = []string{s}
		} else if json.Unmarshal(raw, &o) == nil {
			if len(o.Voices) > 0 { // a model with its own voice list
				nested, _ := json.Marshal(map[string]any{"voices": o.Voices})
				found = parseVoices(nested)
			} else {
				found = []string{firstNonEmpty(o.VoiceID, firstNonEmpty(o.ID, o.Name))}
			}
		}
		for _, v := range found {
			if v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

func shortErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func clip(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}
