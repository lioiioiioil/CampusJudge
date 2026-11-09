package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

const sessionCookie = "campus_session"

func newID(prefix string) string { return prefix + "_" + strings.ToLower(rand.Text()) }
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func (a *App) currentUser(r *http.Request) (model.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || len(c.Value) > 128 {
		return model.User{}, false
	}
	state := a.store.Snapshot()
	hash := tokenHash(c.Value)
	for _, session := range state.Sessions {
		if session.TokenHash == hash && session.ExpiresAt.After(a.now()) {
			for _, u := range state.Users {
				if u.ID == session.UserID {
					return u, true
				}
			}
		}
	}
	return model.User{}, false
}
func newSession(userID string, now time.Time) (string, model.Session) {
	token := rand.Text()
	return token, model.Session{TokenHash: tokenHash(token), UserID: userID, ExpiresAt: now.Add(24 * time.Hour)}
}
func (a *App) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.secureSession(r), MaxAge: 86400})
}
func appendSession(s *model.State, session model.Session, now time.Time) {
	active := make([]model.Session, 0, len(s.Sessions)+1)
	for _, existing := range s.Sessions {
		if existing.ExpiresAt.After(now) {
			active = append(active, existing)
		}
	}
	s.Sessions = append(active, session)
}
func usernameOK(s string) bool {
	if len(s) < 3 || len(s) > 32 {
		return false
	}
	for i, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9' && i > 0) || (i > 0 && (c == '_' || c == '-' || c == '.')) {
			continue
		}
		return false
	}
	return true
}
