package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
	"github.com/lioiioiioil/CampusJudge/internal/testsupport"
)

func postgresClassroom(t *testing.T) (*fixture, string) {
	t.Helper()
	dsn := testsupport.PostgresURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := store.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := Initialize(s, BootstrapOptions{Demo: true, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, app: New(s), store: s}
	response := f.request(http.MethodPost, "/api/login", map[string]string{"username": "teacher", "password": "campus-demo"}, nil, http.StatusOK)
	f.teacher = response.Result().Cookies()[0]
	return f, dsn
}

func reopenPostgresClassroom(t *testing.T, f *fixture, dsn string) {
	t.Helper()
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := store.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f.store, f.app = s, New(s)
}

func TestPostgresTeachingWorkflowAndSessionsSurviveRestart(t *testing.T) {
	f, dsn := postgresClassroom(t)
	_, cookie := f.student("postgreslearner")
	initial := f.store.Snapshot()
	topic := decode[model.Topic](t, f.request(http.MethodPost, "/api/topics", map[string]string{"title": "Циклы", "lesson_url": "https://example.com/lesson"}, f.teacher, http.StatusCreated))
	work := decode[model.Work](t, f.request(http.MethodPost, "/api/works", map[string]any{
		"title": "Работа в PostgreSQL", "kind": "homework", "topic_id": topic.ID,
		"group_ids": initial.Works[0].GroupIDs, "problem_ids": initial.Works[0].ProblemIDs, "status": "draft",
	}, f.teacher, http.StatusCreated))
	before := f.request(http.MethodGet, "/api/state", nil, cookie, http.StatusOK)
	if strings.Contains(before.Body.String(), work.ID) {
		t.Fatal("draft work leaked to student")
	}
	f.request(http.MethodPatch, "/api/works/"+work.ID, map[string]string{"status": "open"}, f.teacher, http.StatusOK)
	const source = "fn main() { println!(\"5\"); }\n"
	submission := decode[model.Submission](t, f.request(http.MethodPost, "/api/submissions", map[string]string{
		"work_id": work.ID, "problem_id": work.ProblemIDs[0], "language": "rust", "source": source,
	}, cookie, http.StatusCreated))
	f.request(http.MethodPatch, "/api/submissions/"+submission.ID, map[string]string{"verdict": "accepted", "feedback": "Проверено преподавателем"}, f.teacher, http.StatusOK)
	reopenPostgresClassroom(t, f, dsn)
	response := f.request(http.MethodGet, "/api/submissions/"+submission.ID, nil, cookie, http.StatusOK)
	got := decode[struct {
		Submission model.Submission `json:"submission"`
	}](t, response).Submission
	if got.Source != source || got.Verdict != "accepted" || got.Feedback != "Проверено преподавателем" || got.ReviewerID == "" {
		t.Fatalf("student source or manual review did not survive restart: %+v", got)
	}
	f.request(http.MethodGet, "/api/state", nil, f.teacher, http.StatusOK)
	studentState := f.request(http.MethodGet, "/api/state", nil, cookie, http.StatusOK)
	for _, private := range []string{"password_hash", "token_hash", `"tests":`} {
		if strings.Contains(studentState.Body.String(), private) {
			t.Fatalf("private field %s leaked from PostgreSQL", private)
		}
	}
	_, otherCookie := f.student("postgresother")
	f.request(http.MethodGet, "/api/submissions/"+submission.ID, nil, otherCookie, http.StatusNotFound)
}

func TestPostgresRunningJudgmentResumesAfterRestart(t *testing.T) {
	f, dsn := postgresClassroom(t)
	_, cookie := f.student("postgresrecovery")
	first := newControlledRunner()
	firstApp := f.app
	cancel := startControlled(t, firstApp, first)
	submission := sendSolution(f, cookie, "go")
	firstJob := takeJob(t, first)
	cancel()
	firstApp.WaitJudge()
	if got := f.store.Snapshot().Submissions[0]; got.Judging == nil || got.Judging.Status != "running" {
		t.Fatal("interrupted judgment was not kept for recovery")
	}
	reopenPostgresClassroom(t, f, dsn)
	second := newControlledRunner()
	startControlled(t, f.app, second)
	resumed := takeJob(t, second)
	if resumed.Source != firstJob.Source || resumed.Language != firstJob.Language || len(resumed.Tests) != len(firstJob.Tests) {
		t.Fatal("restart lost code, language or pinned tests")
	}
	second.replies <- judgeReply{result: judge.Result{Verdict: "accepted", TotalTests: len(resumed.Tests), PassedTests: len(resumed.Tests), TimeMS: 9}}
	final := awaitSubmission(t, f.store, submission.ID, "accepted")
	if final.Judging == nil || final.Judging.Status != "done" || final.Judging.TimeMS != 9 {
		t.Fatalf("recovered judgment did not finish: %+v", final)
	}
}
