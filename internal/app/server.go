package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

type App struct {
	store          *store.Store
	now            func() time.Time
	rateMu         sync.Mutex
	attempts       map[string]*rateEntry
	trustedProxies []netip.Prefix
	runner         judge.Runner
	judgeWake      chan struct{}
	judgeDone      chan struct{}
	auxiliary      auxiliaryRuns
}
type apiError struct {
	code    int
	message string
}

func (e *apiError) Error() string         { return e.message }
func fail(code int, message string) error { return &apiError{code, message} }
func New(s *store.Store, trustedProxies ...netip.Prefix) *App {
	return &App{
		store: s, now: time.Now, attempts: make(map[string]*rateEntry),
		trustedProxies: slices.Clone(trustedProxies),
	}
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.currentUser(r)
		var user any
		if ok {
			user = u.Public()
		}
		writeJSON(w, 200, map[string]any{"user": user, "demo": a.store.Snapshot().Demo})
	})
	mux.HandleFunc("POST /api/login", a.handle(a.login, publicAccess))
	mux.HandleFunc("POST /api/register", a.handle(a.register, publicAccess))
	mux.HandleFunc("POST /api/logout", a.handle(a.logout, signedInAccess))
	mux.HandleFunc("GET /api/state", a.handle(a.state, signedInAccess))
	mux.HandleFunc("POST /api/groups", a.handle(a.createGroup, teacherAccess))
	mux.HandleFunc("POST /api/topics", a.handle(a.createTopic, teacherAccess))
	mux.HandleFunc("POST /api/problems", a.handle(a.createProblem, teacherAccess))
	mux.HandleFunc("POST /api/problems/{id}/test-revisions", a.handle(a.createTestRevision, teacherAccess))
	mux.HandleFunc("POST /api/problems/{id}/reference-check", a.handle(a.referenceCheck, teacherAccess))
	mux.HandleFunc("POST /api/works", a.handle(a.createWork, teacherAccess))
	mux.HandleFunc("PATCH /api/works/{id}", a.handle(a.updateWork, teacherAccess))
	mux.HandleFunc("PATCH /api/works/{id}/problem-revisions", a.handle(a.setWorkTestRevision, teacherAccess))
	mux.HandleFunc("PATCH /api/works/{id}/draft", a.handle(a.editWorkDraft, teacherAccess))
	mux.HandleFunc("POST /api/submissions", a.handle(a.submit, signedInAccess))
	mux.HandleFunc("GET /api/submissions/{id}", a.handle(a.submissionDetails, signedInAccess))
	mux.HandleFunc("GET /api/submission-statuses", a.handle(a.submissionStatuses, signedInAccess))
	mux.HandleFunc("POST /api/trials", a.handle(a.trial, signedInAccess))
	mux.HandleFunc("GET /api/auxiliary-runs/{id}", a.handle(a.auxiliaryResult, signedInAccess))
	mux.HandleFunc("PATCH /api/submissions/{id}", a.handle(a.review, teacherAccess))
	mux.HandleFunc("POST /api/submissions/{id}/retry", a.handle(a.retryJudging, teacherAccess))
	mux.HandleFunc("POST /api/submissions/{id}/rejudge", a.handle(a.rejudgeSubmission, teacherAccess))
	mux.HandleFunc("GET /api/works/{id}/standings", a.handle(a.standings, signedInAccess))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("X-Campus-Request") != "1" {
			writeJSON(w, 403, map[string]string{"error": "Запрос должен быть отправлен из CampusJudge"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// Access policy is declared once at route registration; handlers cannot choose roles.
type accessLevel uint8

const (
	publicAccess accessLevel = iota
	signedInAccess
	teacherAccess
)

type endpoint func(http.ResponseWriter, *http.Request, model.User) error

func (a *App) handle(fn endpoint, access accessLevel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u model.User
		if access != publicAccess {
			var ok bool
			u, ok = a.currentUser(r)
			if !ok {
				writeJSON(w, 401, map[string]string{"error": "Войдите в аккаунт"})
				return
			}
		}
		if access == teacherAccess && u.Role != "teacher" {
			writeJSON(w, 403, map[string]string{"error": "Доступно только преподавателю"})
			return
		}
		if err := fn(w, r, u); err != nil {
			var api *apiError
			if errors.As(err, &api) {
				writeJSON(w, api.code, map[string]string{"error": api.message})
			} else {
				slog.Error("request failed", "path", r.URL.Path, "error", err)
				writeJSON(w, 500, map[string]string{"error": "Не удалось сохранить данные. Попробуйте ещё раз"})
			}
		}
	}
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fail(415, "Нужен запрос в формате JSON")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fail(400, "Некорректные данные или слишком большой запрос")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fail(400, "Ожидался один объект JSON")
	}
	return nil
}
