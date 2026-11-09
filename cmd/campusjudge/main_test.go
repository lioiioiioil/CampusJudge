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
