package main

// First run and pairing. VoiceLoop writes config.json with a random token the first
// time it starts, and the About tab shows that token with the addresses the hook can
// use, so pairing is a copy and paste rather than a file edited by hand.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultConfig is what a first run writes. Kokoro on the same PC, on its usual port.
func defaultConfig() Config {
	return Config{Listen: ":8765", KokoroURL: "http://127.0.0.1:8880", Voice: "bf_emma", Speed: 1.0, PreRollMs: 400}
}

// ensureConfig reads dir/config.json, or writes a new one with a random token if there
// isn't one. created reports whether it was written just now.
func ensureConfig(dir string) (c Config, created bool, err error) {
	p := filepath.Join(dir, "config.json")
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		c = defaultConfig()
		if c.Token, err = newToken(); err != nil {
			return c, false, err
		}
		out, _ := json.MarshalIndent(c, "", "  ")
		if err := os.WriteFile(p, append(out, '\n'), 0o600); err != nil {
			return c, false, fmt.Errorf("writing %s: %w", p, err)
		}
		return c, true, nil
	}
	if err != nil {
		return c, false, err
	}
	c = defaultConfig()
	c.KokoroURL = "" // an existing config.json without one shouldn't gain a Kokoro address
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	if err := json.Unmarshal(b, &c); err != nil {
		return c, false, fmt.Errorf("%s: %w", p, err)
	}
	if c.Token == "" {
		return c, false, fmt.Errorf("%s: token is required", p)
	}
	return c, false, nil
}

func newToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// listenPort is the port part of a listen address such as ":8765" or "0.0.0.0:8765".
func listenPort(listen string) string {
	if _, port, err := net.SplitHostPort(listen); err == nil {
		return port
	}
	return strings.TrimPrefix(listen, ":")
}

// hookURLs lists the addresses a hook on another machine could use to reach this PC:
// one per up, non-loopback IPv4 address, private ranges first.
func hookURLs(listen string) []string {
	port := listenPort(listen)
	var private, public []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			u := "http://" + net.JoinHostPort(ipn.IP.String(), port)
			if ipn.IP.IsPrivate() || isCGNAT(ipn.IP) {
				private = append(private, u)
			} else {
				public = append(public, u)
			}
		}
	}
	sort.Strings(private)
	sort.Strings(public)
	return append(private, public...)
}

// isCGNAT covers 100.64.0.0/10, which Tailscale uses for tailnet addresses.
func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}
