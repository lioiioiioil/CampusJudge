//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailedListenerDoesNotModifyQueue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	before := []byte(`{"version":1,"users":[{"id":"teacher"}],"submissions":[{"id":"active","verdict":"pending","judging":{"status":"running"}}]}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	err := serve(context.Background(), serverConfig{addr: "missing-port", dataPath: path, judging: "manual"})
	if err == nil || !strings.Contains(err.Error(), "listen HTTP") {
		t.Fatalf("startup error = %v, want listener failure", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed startup changed the persisted queue")
	}
}

func TestTrustedProxyConfigurationIsExplicit(t *testing.T) {
	if prefixes, err := parseTrustedProxies(""); err != nil || len(prefixes) != 0 {
		t.Fatal("default configuration trusts a proxy")
	}
	for _, value := range []string{"bad", "0.0.0.0/0", "::/0", "127.0.0.1/32,"} {
		if _, err := parseTrustedProxies(value); err == nil {
			t.Errorf("accepted proxy configuration %q", value)
		}
	}
	prefixes, err := parseTrustedProxies("127.0.0.1/32, ::1/128")
	if err != nil || len(prefixes) != 2 {
		t.Fatalf("loopback proxies were not parsed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	err = serve(context.Background(), serverConfig{
		addr: "127.0.0.1:0", dataPath: path, demo: true, judging: "manual", trustedProxies: prefixes,
	})
	if err == nil || !strings.Contains(err.Error(), "non-demo state") {
		t.Fatalf("demo server accepted trusted proxies: %v", err)
	}
}
