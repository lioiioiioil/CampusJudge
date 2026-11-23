package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/app"
)

func TestHealthcheckDoesNotOpenStorageOrConfigureJudge(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/healthz" {
			t.Errorf("unexpected probe: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	t.Setenv("CAMPUS_DATABASE_URL", "invalid database URL")
	t.Setenv("HTTP_PROXY", "http://192.0.2.1:1")
	path := filepath.Join(t.TempDir(), "must-not-open.json")
	err := runArgs([]string{"-healthcheck", "-addr", strings.TrimPrefix(server.URL, "http://"), "-data", path, "-trusted-proxies", "invalid", "-judge", "unavailable"})
	if err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("healthcheck requests = %d, want 1", got)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatal("healthcheck opened storage")
	}
}

func TestHealthcheckRejectsUnhealthyRedirectAndExternalAddress(t *testing.T) {
	for _, response := range []struct {
		name string
		code int
		body string
	}{
		{name: "unavailable", code: http.StatusServiceUnavailable, body: `{"status":"ok"}`},
		{name: "invalid body", code: http.StatusOK, body: `not json`},
		{name: "wrong status", code: http.StatusOK, body: `{"status":"failed"}`},
		{name: "redirect", code: http.StatusTemporaryRedirect, body: `{"status":"ok"}`},
	} {
		t.Run(response.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "http://192.0.2.1/never-contact")
				w.WriteHeader(response.code)
				_, _ = w.Write([]byte(response.body))
			}))
			defer server.Close()
			if err := checkHealth(strings.TrimPrefix(server.URL, "http://")); err == nil {
				t.Fatal("unhealthy server accepted")
			}
		})
	}
	for _, addr := range []string{"192.0.2.1:8080", "example.com:8080", "missing-port"} {
		if err := checkHealth(addr); err == nil {
			t.Fatalf("invalid or external probe address %q accepted", addr)
		}
	}
}

func TestBootstrapDoesNotLogConfiguredTeacherPassword(t *testing.T) {
	bootstrap := app.Bootstrap{Username: "teacher", Password: "configured-private-password", InviteCode: "CLASS"}
	var output bytes.Buffer
	printBootstrap(&output, bootstrap, false)
	if strings.Contains(output.String(), bootstrap.Password) {
		t.Fatal("configured teacher password exposed in container logs")
	}
	if !strings.Contains(output.String(), bootstrap.Username) || !strings.Contains(output.String(), bootstrap.InviteCode) {
		t.Fatal("bootstrap omitted account or registration instructions")
	}
	output.Reset()
	printBootstrap(&output, bootstrap, true)
	if !strings.Contains(output.String(), bootstrap.Password) {
		t.Fatal("new generated password was not shown to its owner")
	}
}
