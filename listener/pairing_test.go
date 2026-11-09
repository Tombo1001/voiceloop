package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureConfigFirstRun(t *testing.T) {
	dir := t.TempDir()
	c, created, err := ensureConfig(dir)
	if err != nil || !created {
		t.Fatalf("first run: created=%v err=%v", created, err)
	}
	if len(c.Token) != 48 || c.Listen != ":8765" || c.KokoroURL == "" {
		t.Fatalf("first-run config wrong: %+v", c)
	}
	again, created, err := ensureConfig(dir)
	if err != nil || created || again.Token != c.Token {
		t.Fatalf("second run should read the same token: created=%v err=%v %+v", created, err, again)
	}
}

func TestEnsureConfigExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	// A BOM, as PowerShell's Set-Content -Encoding UTF8 writes.
	os.WriteFile(p, []byte("\ufeff{\"listen\": \":9000\", \"token\": \"abc\"}"), 0o600)
	c, created, err := ensureConfig(dir)
	if err != nil || created || c.Token != "abc" || c.Listen != ":9000" || c.KokoroURL != "" {
		t.Fatalf("existing config: created=%v err=%v %+v", created, err, c)
	}
	os.WriteFile(p, []byte(`{"listen": ":9000"}`), 0o600)
	if _, _, err := ensureConfig(dir); err == nil {
		t.Fatal("a config.json without a token should be refused")
	}
}

func TestListenPortAndCGNAT(t *testing.T) {
	for in, want := range map[string]string{":8765": "8765", "0.0.0.0:9000": "9000", "8765": "8765"} {
		if got := listenPort(in); got != want {
			t.Errorf("listenPort(%q) = %q, want %q", in, got, want)
		}
	}
	if !isCGNAT(net.ParseIP("100.64.1.2")) || isCGNAT(net.ParseIP("100.128.0.1")) || isCGNAT(net.ParseIP("10.0.0.1")) {
		t.Error("isCGNAT wrong")
	}
}
