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

func TestInvalidStorageConfigurationDoesNotCreateJSONData(t *testing.T) {
	for _, test := range []struct {
		name   string
		config serverConfig
		want   string
	}{
		{
			name:   "explicit JSON path with database",
			config: serverConfig{databaseURL: "postgres://localhost/campusjudge", dataExplicit: true},
			want:   "-data selects JSON storage",
		},
		{
			name:   "import without database",
			config: serverConfig{importJSON: "existing.json"},
			want:   "-import-json requires CAMPUS_DATABASE_URL",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			config := test.config
			config.dataPath = path
			config.judging = "manual"
			err := serve(context.Background(), config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("startup error = %v, want %q", err, test.want)
			}
			if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
				t.Fatalf("invalid configuration touched JSON storage: %v", err)
			}
		})
	}
}

func TestFailedDatabaseDoesNotFallBackToJSONOrExposeCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	const password = "private-connection-password"
	err := serve(context.Background(), serverConfig{
		addr:        "127.0.0.1:0",
		dataPath:    path,
		databaseURL: "postgres://teacher:" + password + "@localhost:invalid/campusjudge",
		judging:     "manual",
	})
	if err == nil {
		t.Fatal("invalid database configuration started successfully")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatal("database startup error exposes connection password")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed database created JSON data: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("failed database acquired JSON ownership: %v", err)
	}
}
