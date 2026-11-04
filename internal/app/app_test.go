package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

type fixture struct {
	t       *testing.T
	app     *App
	store   *store.Store
	path    string
	teacher *http.Cookie
}

func setup(t *testing.T) *fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(s, true); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, app: New(s), store: s, path: path}
	response := f.request("POST", "/api/login", map[string]string{"username": "teacher", "password": "campus-demo"}, nil, 200)
	f.teacher = response.Result().Cookies()[0]
	return f
}
func (f *fixture) request(method, path string, body any, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
	f.t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Campus-Request", "1")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, r)
	if w.Code != want {
		f.t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	return w
}
func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func (f *fixture) student(username string) (model.PublicUser, *http.Cookie) {
	f.t.Helper()
	w := f.request("POST", "/api/register", map[string]string{"name": username, "username": username, "password": "student-password", "invite_code": "DEMOCLASS"}, nil, 201)
	result := decode[struct {
		User model.PublicUser `json:"user"`
	}](f.t, w)
	return result.User, w.Result().Cookies()[0]
}
func TestMinimalSeedAndTeachingWorkflow(t *testing.T) {
	f := setup(t)
	initial := f.store.Snapshot()
	if len(initial.Users) != 1 || len(initial.Topics) != 0 || len(initial.Works) != 2 || len(initial.Submissions) != 0 {
		t.Fatalf("unexpected initial content: %+v", initial)
	}
	topicResponse := f.request("POST", "/api/topics", map[string]string{"title": "Циклы", "description": "Материалы урока", "lesson_url": "https://telemost.yandex.ru/j/example"}, f.teacher, 201)
	topic := decode[model.Topic](t, topicResponse)
	if topic.LessonURL != "https://telemost.yandex.ru/j/example" {
		t.Fatal("lesson URL was not saved")
	}
	homework := initial.Works[0]
	created := f.request("POST", "/api/works", map[string]any{"title": "После урока", "kind": "homework", "topic_id": topic.ID, "group_ids": homework.GroupIDs, "problem_ids": homework.ProblemIDs, "status": "draft"}, f.teacher, 201)
	draft := decode[model.Work](t, created)
	student, cookie := f.student("learner")
	response := f.request("GET", "/api/state", nil, cookie, 200)
	var state struct {
		Works    []model.Work       `json:"works"`
		Problems []model.Problem    `json:"problems"`
		Groups   []model.Group      `json:"groups"`
		Users    []model.PublicUser `json:"users"`
	}
	state = decode[struct {
		Works    []model.Work       `json:"works"`
		Problems []model.Problem    `json:"problems"`
		Groups   []model.Group      `json:"groups"`
		Users    []model.PublicUser `json:"users"`
	}](t, response)
	if !strings.Contains(response.Body.String(), topic.LessonURL) {
		t.Fatal("lesson URL must be available before homework publication")
	}
	for _, work := range state.Works {
		if work.ID == draft.ID {
			t.Fatal("draft leaked to student")
		}
	}
	for _, p := range state.Problems {
		if len(p.Tests) > 0 {
			t.Fatal("hidden tests leaked")
		}
	}
	if len(state.Users) != 1 || state.Users[0].ID != student.ID || state.Groups[0].InviteCode != "" {
		t.Fatal("private account/group fields leaked")
	}
	if strings.Contains(response.Body.String(), "password_hash") || strings.Contains(response.Body.String(), "token_hash") {
		t.Fatal("credentials leaked")
	}
	f.request("POST", "/api/topics", map[string]string{"title": "Forbidden"}, cookie, 403)
	f.request("PATCH", "/api/works/"+draft.ID, map[string]string{"status": "open"}, cookie, 403)
	f.request("POST", "/api/submissions", map[string]string{"work_id": draft.ID, "problem_id": draft.ProblemIDs[0], "language": "python", "source": "print(5)"}, cookie, 404)
	f.request("PATCH", "/api/works/"+draft.ID, map[string]string{"status": "open"}, f.teacher, 200)
	sent := f.request("POST", "/api/submissions", map[string]string{"work_id": draft.ID, "problem_id": draft.ProblemIDs[0], "language": "python", "source": "a,b=map(int,input().split())\nprint(a+b)"}, cookie, 201)
	submission := decode[model.Submission](t, sent)
	if submission.Verdict != "pending" || submission.UserID != student.ID {
		t.Fatal("unexpected submission")
	}
	f.request("PATCH", "/api/submissions/"+submission.ID, map[string]string{"verdict": "accepted"}, cookie, 403)
	f.request("PATCH", "/api/submissions/"+submission.ID, map[string]string{"verdict": "accepted", "feedback": "Верно"}, f.teacher, 200)
	reopened, err := store.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.app = New(reopened)
	checked := f.request("GET", "/api/state", nil, cookie, 200)
	if !strings.Contains(checked.Body.String(), "Верно") || !strings.Contains(checked.Body.String(), "a+b") {
		t.Fatal("submission or feedback not persisted across restart")
	}
	_, secondCookie := f.student("second")
	otherState := f.request("GET", "/api/state", nil, secondCookie, 200)
	if strings.Contains(otherState.Body.String(), submission.ID) || strings.Contains(otherState.Body.String(), "a+b") {
		t.Fatal("another student's source leaked")
	}
}
func TestGroupsAndPublicationAreEnforced(t *testing.T) {
	f := setup(t)
	_, cookie := f.student("learner")
	group := decode[model.Group](t, f.request("POST", "/api/groups", map[string]string{"name": "Другая группа"}, f.teacher, 201))
	problemID := f.store.Snapshot().Problems[0].ID
	work := decode[model.Work](t, f.request("POST", "/api/works", map[string]any{"title": "Чужая работа", "kind": "homework", "status": "open", "group_ids": []string{group.ID}, "problem_ids": []string{problemID}}, f.teacher, 201))
	result := f.request("GET", "/api/state", nil, cookie, 200)
	if strings.Contains(result.Body.String(), work.ID) || strings.Contains(result.Body.String(), group.InviteCode) {
		t.Fatal("another group leaked")
	}
	f.request("GET", "/api/works/"+work.ID+"/standings", nil, cookie, 404)
	f.request("POST", "/api/submissions", map[string]string{"work_id": work.ID, "problem_id": problemID, "language": "go", "source": "package main"}, cookie, 404)
	f.request("POST", "/api/works", map[string]any{"title": "Пустая работа", "kind": "homework", "status": "open", "group_ids": []string{}, "problem_ids": []string{}}, f.teacher, 400)
	f.request("POST", "/api/works", map[string]any{"title": "Неверная группа", "kind": "homework", "status": "open", "group_ids": []string{"missing"}, "problem_ids": []string{problemID}}, f.teacher, 400)
	f.request("POST", "/api/works", map[string]any{"title": "Нет времени", "kind": "contest", "status": "draft"}, f.teacher, 400)
	f.request("POST", "/api/topics", map[string]string{"title": "Опасная ссылка", "lesson_url": "javascript:alert(1)"}, f.teacher, 400)
	f.request("POST", "/api/register", map[string]string{"name": "Admin", "username": "admin", "password": "student-password", "invite_code": "DEMOCLASS", "role": "teacher"}, nil, 400)
}
func TestContestTimeBoundariesAndScore(t *testing.T) {
	f := setup(t)
	user, cookie := f.student("learner")
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	f.app.now = func() time.Time { return now }
	state := f.store.Snapshot()
	contest := state.Works[1]
	start := now.Add(time.Hour)
	end := start.Add(time.Hour)
	if err := f.store.Update(func(s *model.State) error { s.Works[1].StartsAt = &start; s.Works[1].EndsAt = &end; return nil }); err != nil {
		t.Fatal(err)
	}
	before := f.request("GET", "/api/state", nil, cookie, 200)
	if strings.Contains(before.Body.String(), state.Problems[1].Statement) {
		t.Fatal("future contest statement leaked")
	}
	payload := map[string]string{"work_id": contest.ID, "problem_id": contest.ProblemIDs[0], "language": "cpp", "source": "int main() {}"}
	f.request("POST", "/api/submissions", payload, cookie, 403)
	now = start
	first := decode[model.Submission](t, f.request("POST", "/api/submissions", payload, cookie, 201))
	f.request("PATCH", "/api/submissions/"+first.ID, map[string]string{"verdict": "wrong_answer"}, f.teacher, 200)
	now = start.Add(5 * time.Minute)
	second := decode[model.Submission](t, f.request("POST", "/api/submissions", payload, cookie, 201))
	f.request("PATCH", "/api/submissions/"+second.ID, map[string]string{"verdict": "compilation_error"}, f.teacher, 200)
	now = start.Add(10 * time.Minute)
	third := decode[model.Submission](t, f.request("POST", "/api/submissions", payload, cookie, 201))
	f.request("PATCH", "/api/submissions/"+third.ID, map[string]string{"verdict": "accepted"}, f.teacher, 200)
	result := decode[struct {
		Rows []standingRow `json:"rows"`
	}](t, f.request("GET", "/api/works/"+contest.ID+"/standings", nil, cookie, 200))
	if len(result.Rows) != 1 || result.Rows[0].UserID != user.ID || result.Rows[0].Solved != 1 || result.Rows[0].Penalty != 30 {
		t.Fatalf("expected one solved at minute 10 plus one 20-minute penalty: %+v", result.Rows)
	}
	now = end
	f.request("POST", "/api/submissions", payload, cookie, 403)
	f.request("PATCH", "/api/works/"+state.Works[0].ID, map[string]string{"status": "closed"}, f.teacher, 200)
	payload["work_id"] = state.Works[0].ID
	payload["problem_id"] = state.Works[0].ProblemIDs[0]
	f.request("POST", "/api/submissions", payload, cookie, 403)
}
func TestAuthenticationAndRequestProtection(t *testing.T) {
	f := setup(t)
	f.request("GET", "/api/state", nil, nil, 401)
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"teacher","password":"campus-demo"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.app.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("missing CSRF request marker accepted: %d", w.Code)
	}
	f.request("POST", "/api/login", map[string]string{"username": "teacher", "password": "wrong"}, nil, 401)
	f.request("POST", "/api/register", map[string]string{"name": "Student", "username": "newstudent", "password": "student-password", "invite_code": "wrong"}, nil, 400)
	f.request("POST", "/api/logout", map[string]any{}, f.teacher, 200)
	f.request("GET", "/api/state", nil, f.teacher, 401)
}
