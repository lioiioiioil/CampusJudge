package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"
)

// A probe can only contact this host. Disable environment proxies so a local
// container healthcheck never sends requests or credentials to an external proxy.
func checkHealth(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return errors.New("healthcheck requires a loopback HTTP address")
	}
	if host == "" || host == "0.0.0.0" || host == "localhost" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("healthcheck requires a loopback HTTP address")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return errors.New("invalid healthcheck address")
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		Timeout:       3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("local HTTP healthcheck failed")
	}
	defer response.Body.Close()
	var status struct {
		Status string `json:"status"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&status) != nil || status.Status != "ok" {
		return errors.New("local HTTP server is unhealthy")
	}
	return nil
}
