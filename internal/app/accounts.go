package app

import (
	"net/http"
	"strings"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) login(w http.ResponseWriter, r *http.Request, _ model.User) error {
	if !a.allowAuth(r) {
		return fail(429, "Слишком много попыток. Подождите минуту")
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if len(in.Password) > 128 {
		return fail(401, "Неверный логин или пароль")
	}
	var found model.User
	for _, u := range a.store.Snapshot().Users {
		if u.Username == username {
			found = u
			break
		}
	}
	if found.ID == "" || !passwordMatches(found.PasswordHash, in.Password) {
		return fail(401, "Неверный логин или пароль")
	}
	now := a.now()
	token, session := newSession(found.ID, now)
	if err := a.store.Update(func(s *model.State) error { appendSession(s, session, now); return nil }); err != nil {
		return err
	}
	setSessionCookie(w, r, token)
	writeJSON(w, 200, map[string]any{"user": found.Public()})
	return nil
}
func (a *App) register(w http.ResponseWriter, r *http.Request, _ model.User) error {
	if !a.allowAuth(r) {
		return fail(429, "Слишком много попыток. Подождите минуту")
	}
	var in struct {
		Name       string `json:"name"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.InviteCode = strings.ToUpper(strings.TrimSpace(in.InviteCode))
	if !usernameOK(in.Username) {
		return fail(400, "Логин: 3–32 латинских символа, первая буква, далее буквы, цифры, . _ -")
	}
	if len(in.Name) < 2 || len(in.Name) > 100 {
		return fail(400, "Укажите имя от 2 до 100 символов")
	}
	if len(in.Password) < 8 || len(in.Password) > 128 {
		return fail(400, "Пароль должен содержать от 8 до 128 символов")
	}
	encodedPassword, err := passwordHash(in.Password)
	if err != nil {
		return err
	}
	u := model.User{ID: newID("user"), Name: in.Name, Username: in.Username, Role: "student", PasswordHash: encodedPassword}
	now := a.now()
	token, session := newSession(u.ID, now)
	err = a.store.Update(func(s *model.State) error {
		for _, existing := range s.Users {
			if existing.Username == u.Username {
				return fail(409, "Такой логин уже занят")
			}
		}
		for _, g := range s.Groups {
			if g.InviteCode == in.InviteCode && in.InviteCode != "" {
				u.GroupID = g.ID
				break
			}
		}
		if u.GroupID == "" {
			return fail(400, "Код приглашения не найден. Уточните его у преподавателя")
		}
		s.Users = append(s.Users, u)
		appendSession(s, session, now)
		return nil
	})
	if err != nil {
		return err
	}
	setSessionCookie(w, r, token)
	writeJSON(w, 201, map[string]any{"user": u.Public()})
	return nil
}
func (a *App) logout(w http.ResponseWriter, r *http.Request, _ model.User) error {
	c, _ := r.Cookie(sessionCookie)
	hash := tokenHash(c.Value)
	if err := a.store.Update(func(s *model.State) error {
		sessions := make([]model.Session, 0, len(s.Sessions))
		for _, v := range s.Sessions {
			if v.TokenHash != hash {
				sessions = append(sessions, v)
			}
		}
		s.Sessions = sessions
		return nil
	}); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
	writeJSON(w, 200, map[string]bool{"ok": true})
	return nil
}
