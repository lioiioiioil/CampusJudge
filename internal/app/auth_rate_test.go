package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func authPost(handler http.Handler, path, body, peer string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.RemoteAddr = peer
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Campus-Request", "1")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestClassCanLogInConcurrentlyFromOneAddress(t *testing.T) {
	f := setup(t)
	now := time.Now()
	f.app.now = func() time.Time { return now }
	const participants = 20
	if err := f.store.Update(func(state *model.State) error {
		hash := state.Users[0].PasswordHash
		for i := range participants {
			username := fmt.Sprintf("student%d", i)
			state.Users = append(state.Users, model.User{ID: username, Username: username, Role: "student", PasswordHash: hash})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := f.app.Handler()
	statuses := make([]int, participants)
	var requests sync.WaitGroup
	for i := range participants {
		requests.Go(func() {
			body := fmt.Sprintf(`{"username":"student%d","password":"campus-demo"}`, i)
			statuses[i] = authPost(handler, "/api/login", body, "192.0.2.1:1234", nil).Code
		})
	}
	requests.Wait()
	for i, status := range statuses {
		if status != http.StatusOK {
			t.Errorf("student %d login status = %d, want 200", i, status)
		}
	}
	if got := len(f.store.Snapshot().Sessions); got != participants+1 {
		t.Errorf("stored sessions = %d, want %d", got, participants+1)
	}
}

func TestFailedLoginLimitFollowsAccountAcrossOrigins(t *testing.T) {
	f := setup(t)
	now := time.Now()
	f.app.now = func() time.Time { return now }
	handler := f.app.Handler()
	for i := range 6 {
		peer := fmt.Sprintf("192.0.2.%d:1234", i+1)
		if got := authPost(handler, "/api/login", `{"username":" Teacher ","password":"wrong"}`, peer, nil).Code; got != 401 {
			t.Fatalf("guess %d returned %d, want 401", i+1, got)
		}
	}
	const valid = `{"username":"teacher","password":"campus-demo"}`
	if got := authPost(handler, "/api/login", valid, "192.0.2.90:1234", nil).Code; got != 429 {
		t.Errorf("same account from a new origin returned %d, want 429", got)
	}
	now = now.Add(time.Minute)
	if got := authPost(handler, "/api/login", valid, "192.0.2.90:1234", nil).Code; got != 200 {
		t.Errorf("login after expiry returned %d, want 200", got)
	}
}

func TestOriginFailureBudgetIgnoresUntrustedHeadersAndKeepsRegistrationSeparate(t *testing.T) {
	f := setup(t)
	now := time.Now()
	f.app.now = func() time.Time { return now }
	handler := f.app.Handler()
	const peer = "198.51.100.20:1234"
	for i := range 32 {
		body := fmt.Sprintf(`{"username":"missing%d","password":"wrong"}`, i)
		headers := map[string]string{"X-Forwarded-For": fmt.Sprintf("192.0.2.%d", i+1)}
		if got := authPost(handler, "/api/login", body, peer, headers).Code; got != 401 {
			t.Fatalf("guess %d returned %d, want 401", i+1, got)
		}
	}
	headers := map[string]string{"X-Forwarded-For": "192.0.2.99"}
	const valid = `{"username":"teacher","password":"campus-demo"}`
	if got := authPost(handler, "/api/login", valid, peer, headers).Code; got != 429 {
		t.Errorf("untrusted X-Forwarded-For changed the rate key: status %d", got)
	}
	if got := authPost(handler, "/api/register", `{}`, peer, nil).Code; got != 400 {
		t.Errorf("login failures charged the registration budget: status %d", got)
	}
	const anotherPeer = "198.51.100.21:1234"
	for range 32 {
		if got := authPost(handler, "/api/register", `{}`, anotherPeer, nil).Code; got != 400 {
			t.Fatalf("registration returned %d before the configured limit", got)
		}
	}
	if got := authPost(handler, "/api/register", `{}`, anotherPeer, nil).Code; got != 429 {
		t.Errorf("registration limit returned %d, want 429", got)
	}
	if got := authPost(handler, "/api/login", valid, anotherPeer, nil).Code; got != 200 {
		t.Errorf("registration requests charged the login budget: status %d", got)
	}
}

func TestTrustedProxyUsesLastUntrustedHopAndSetsSecureCookie(t *testing.T) {
	f := setup(t)
	a := New(f.store, netip.MustParsePrefix("10.1.2.0/24"))
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.RemoteAddr = "10.1.2.5:9000"
	request.Header.Add("X-Forwarded-For", "192.0.2.99, 198.51.100.80")
	request.Header.Add("X-Forwarded-For", "10.1.2.4")
	if got := a.requestOrigin(request); got != "198.51.100.80" {
		t.Errorf("proxied origin = %q, want the untrusted hop nearest the proxy", got)
	}
	request.Header.Set("X-Forwarded-For", "192.0.2.99, invalid")
	if got := a.requestOrigin(request); got != "10.1.2.5" {
		t.Errorf("malformed forwarded address = %q, want the peer", got)
	}
	const valid = `{"username":"teacher","password":"campus-demo"}`
	headers := map[string]string{"X-Forwarded-For": "198.51.100.80", "X-Forwarded-Proto": "https"}
	handler := a.Handler()
	for _, tc := range []struct {
		peer   string
		secure bool
	}{
		{peer: "10.1.2.5:9000", secure: true},
		{peer: "198.51.100.90:9000", secure: false},
	} {
		response := authPost(handler, "/api/login", valid, tc.peer, headers)
		cookies := response.Result().Cookies()
		if response.Code != http.StatusOK || len(cookies) != 1 || cookies[0].Secure != tc.secure {
			t.Errorf("login from %s returned status %d, want a session cookie with Secure=%t", tc.peer, response.Code, tc.secure)
		}
	}
}

func TestLateSuccessfulLoginCannotRefundNewWindow(t *testing.T) {
	a := New(nil)
	now := time.Now()
	a.now = func() time.Time { return now }
	budget := authBudget{key: "account", limit: 1, refundOnSuccess: true}
	refund, ok := a.reserveAuth(budget)
	if !ok {
		t.Fatal("first window did not admit an attempt")
	}
	now = now.Add(time.Minute)
	if _, ok := a.reserveAuth(budget); !ok {
		t.Fatal("new window did not admit an attempt")
	}
	refund()
	if _, ok := a.reserveAuth(budget); ok {
		t.Fatal("late success refunded an attempt in a different window")
	}
}
