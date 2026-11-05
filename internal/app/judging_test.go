package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

type judgeReply struct {
	result judge.Result
	err    error
}
type controlledRunner struct {
	calls   chan judge.Job
	replies chan judgeReply
}

func newControlledRunner() *controlledRunner {
	return &controlledRunner{make(chan judge.Job, 10), make(chan judgeReply, 10)}
}
func (r *controlledRunner) Run(ctx context.Context, job judge.Job) (judge.Result, error) {
	select {
	case r.calls <- job:
	case <-ctx.Done():
		return judge.Result{}, ctx.Err()
	}
	select {
	case reply := <-r.replies:
		return reply.result, reply.err
	case <-ctx.Done():
		return judge.Result{}, ctx.Err()
	}
}
func startControlled(t *testing.T, a *App, runner *controlledRunner) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if err := a.StartJudge(ctx, runner); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); a.WaitJudge() })
	return cancel
}
func takeJob(t *testing.T, runner *controlledRunner) judge.Job {
	t.Helper()
	select {
	case job := <-runner.calls:
		return job
	case <-time.After(3 * time.Second):
		t.Fatal("judge did not claim durable job")
		return judge.Job{}
	}
}
func sendSolution(f *fixture, cookie *http.Cookie, language string) model.Submission {
	f.t.Helper()
	work := f.store.Snapshot().Works[0]
	return decode[model.Submission](f.t, f.request("POST", "/api/submissions", map[string]string{
		"work_id": work.ID, "problem_id": work.ProblemIDs[0], "language": language, "source": "student source\n",
	}, cookie, http.StatusCreated))
}
func awaitSubmission(t *testing.T, s *store.Store, id, verdict string) model.Submission {
	t.Helper()
	deadline := time.NewTimer(4 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		for _, sub := range s.Snapshot().Submissions {
			if sub.ID == id && sub.Verdict == verdict {
				return sub
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("submission %s did not reach %s", id, verdict)
			return model.Submission{}
		case <-tick.C:
		}
	}
}

func TestAutomaticSubmissionLifecycleAndPublicResult(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("automatic")
	sub := sendSolution(f, cookie, "cpp")
	if sub.Verdict != "pending" || sub.Judging == nil || sub.Judging.Status != "queued" {
		t.Fatalf("not queued: %+v", sub)
	}
	job := takeJob(t, runner)
	if job.Language != "cpp" || job.Source != "student source\n" || len(job.Tests) != 3 || job.Tests[1].Input != "-5 8\n" {
		t.Fatalf("job lost samples, hidden tests or original code: %+v", job)
	}
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: 3, TotalTests: 3, TimeMS: 7}}
	final := awaitSubmission(t, f.store, sub.ID, "accepted")
	if final.Judging.Status != "done" || final.JudgedAt == nil || final.ReviewerID != "" || final.Judging.PassedTests != 3 {
		t.Fatalf("bad final result: %+v", final)
	}
	response := f.request("GET", "/api/state", nil, cookie, 200)
	if !strings.Contains(response.Body.String(), `"judging_mode":"automatic"`) || !strings.Contains(response.Body.String(), `"passed_tests":3`) || strings.Contains(response.Body.String(), `"tests":`) {
		t.Fatal("public result/mode missing, or hidden tests disclosed")
	}
	reopened, err := store.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().Submissions[0]; got.Verdict != "accepted" || got.Judging.TimeMS != 7 {
		t.Fatal("automatic result not durable")
	}
}

func TestManualReviewWinsOverLateAutomaticResult(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("override")
	sub := sendSolution(f, cookie, "python")
	job := takeJob(t, runner)
	f.request("PATCH", "/api/submissions/"+sub.ID, map[string]string{"verdict": "wrong_answer", "feedback": "Ручной разбор"}, f.teacher, 200)
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: len(job.Tests), TotalTests: len(job.Tests)}}
	// A second job is an observable barrier: the first result commit has finished.
	_ = sendSolution(f, cookie, "cpp")
	_ = takeJob(t, runner)
	final := awaitSubmission(t, f.store, sub.ID, "wrong_answer")
	if final.Judging != nil || final.ReviewerID == "" || final.Feedback != "Ручной разбор" {
		t.Fatal("automatic result overwrote teacher")
	}
}

func TestRestartResumesRunningJobWithoutAdoptingUnsupportedManualSubmissions(t *testing.T) {
	f := setup(t)
	_, cookie := f.student("recovery")
	manual := sendSolution(f, cookie, "rust")
	first := newControlledRunner()
	cancel := startControlled(t, f.app, first)
	auto := sendSolution(f, cookie, "go")
	_ = takeJob(t, first)
	cancel()
	f.app.WaitJudge()
	reopened, err := store.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	second := newControlledRunner()
	restarted := New(reopened)
	startControlled(t, restarted, second)
	job := takeJob(t, second)
	if job.Language != "go" {
		t.Fatal("recovery adopted a manual submission")
	}
	second.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: 3, TotalTests: 3}}
	_ = awaitSubmission(t, reopened, auto.ID, "accepted")
	for _, sub := range reopened.Snapshot().Submissions {
		if sub.ID == manual.ID && (sub.Verdict != "pending" || sub.Judging != nil) {
			t.Fatal("manual submission changed")
		}
	}
}

func TestEnablingAutomaticJudgeAdoptsOldPendingSolutionsOnly(t *testing.T) {
	f := setup(t)
	_, cookie := f.student("upgrade")
	oldPending := sendSolution(f, cookie, "python")
	oldReviewed := sendSolution(f, cookie, "cpp")
	f.request("PATCH", "/api/submissions/"+oldReviewed.ID, map[string]string{"verdict": "wrong_answer", "feedback": "Сохранить ручной результат"}, f.teacher, 200)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	job := takeJob(t, runner)
	if job.Language != "python" || job.Source != "student source\n" {
		t.Fatal("wrong legacy submission adopted")
	}
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: 3, TotalTests: 3}}
	_ = awaitSubmission(t, f.store, oldPending.ID, "accepted")
	for _, sub := range f.store.Snapshot().Submissions {
		if sub.ID == oldReviewed.ID && (sub.Verdict != "wrong_answer" || sub.Judging != nil || sub.Feedback != "Сохранить ручной результат") {
			t.Fatal("upgrade overwrote teacher result")
		}
	}
	select {
	case <-runner.calls:
		t.Fatal("reviewed submission was executed")
	default:
	}
}

func TestLegacyAdmissionRespectsCapacityAndFillsFreedSlots(t *testing.T) {
	state := model.State{Problems: []model.Problem{{ID: "p", Samples: []model.TestCase{{Output: "1"}}}}}
	for range 5 {
		state.Submissions = append(state.Submissions, model.Submission{UserID: "u", ProblemID: "p", Language: "python", Source: "print(1)", Verdict: "pending"})
	}
	if !prepareQueue(&state, false) {
		t.Fatal("legacy pending jobs not adopted")
	}
	for i, sub := range state.Submissions {
		if (sub.Judging != nil) != (i < 3) {
			t.Fatal("legacy jobs exceeded per-student bound or changed arrival order")
		}
	}
	state.Submissions[0].Verdict = "accepted"
	state.Submissions[0].Judging.Status = "done"
	if !prepareQueue(&state, false) || state.Submissions[3].Judging == nil || state.Submissions[4].Judging != nil {
		t.Fatal("freed slot not filled within bound")
	}
}

func TestLegacyBacklogIsAdmittedBetweenJobsWithoutWaitingForIdle(t *testing.T) {
	f := setup(t)
	_, cookie := f.student("backlog")
	var old []model.Submission
	for range 4 {
		old = append(old, sendSolution(f, cookie, "python"))
	}
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_ = takeJob(t, runner)
	if f.store.Snapshot().Submissions[3].Judging != nil {
		t.Fatal("initial migration exceeded per-user bound")
	}
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: 3, TotalTests: 3}}
	_ = takeJob(t, runner) // Remaining queue is still nonempty; consumer never went idle.
	for _, sub := range f.store.Snapshot().Submissions {
		if sub.ID == old[3].ID && sub.Judging == nil {
			t.Fatal("old backlog waits for idle while newer queue is being drained")
		}
	}
}

func TestInfrastructureFailureIsNotStudentFailure(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("failure")
	sub := sendSolution(f, cookie, "java")
	_ = takeJob(t, runner)
	runner.replies <- judgeReply{err: errors.New("private infrastructure detail")}
	final := awaitSubmission(t, f.store, sub.ID, "judge_error")
	if final.Judging.Status != "error" || final.Judging.Error == "" || strings.Contains(final.Judging.Error, "private") {
		t.Fatal("incorrect public infrastructure error")
	}
}

func TestQueueAdmissionBoundedAndUnsupportedLanguagesStayManual(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("queue")
	_ = sendSolution(f, cookie, "cpp")
	_ = takeJob(t, runner)
	_ = sendSolution(f, cookie, "cpp")
	_ = sendSolution(f, cookie, "cpp")
	work := f.store.Snapshot().Works[0]
	f.request("POST", "/api/submissions", map[string]string{"work_id": work.ID, "problem_id": work.ProblemIDs[0], "language": "cpp", "source": "code"}, cookie, 429)
	manual := sendSolution(f, cookie, "kotlin")
	if manual.Judging != nil {
		t.Fatal("unsupported compiler was advertised as automatic")
	}
}

func TestLanguageTemplatesAndExplicitManualMode(t *testing.T) {
	f := setup(t)
	catalog := languageCatalog(true)
	for _, language := range catalog {
		if language.ID != "other" && language.Template == "" {
			t.Fatalf("missing template: %s", language.ID)
		}
		if language.ID == "cpp" && !strings.Contains(language.Template, "int main()") {
			t.Fatal("C++ needs full main template")
		}
		if language.Automatic != automaticLanguage(language.ID) {
			t.Fatal("language capability inconsistent")
		}
	}
	if err := f.store.Update(func(s *model.State) error {
		s.Submissions = append(s.Submissions, model.Submission{ID: "old", Verdict: "pending", Judging: &model.Judging{Status: "running"}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.UseManualJudging(); err != nil {
		t.Fatal(err)
	}
	got := f.store.Snapshot().Submissions[0]
	if got.Verdict != "judge_error" || got.Judging.Status != "error" {
		t.Fatal("manual mode left phantom running job")
	}
	for _, language := range languageCatalog(false) {
		if language.Automatic {
			t.Fatal("manual mode advertises automatic compiler")
		}
	}
}

func TestAutomaticAdmissionRejectsOversizedSourceAndMissingTests(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("admission")
	work := f.store.Snapshot().Works[0]
	body := map[string]string{"work_id": work.ID, "problem_id": work.ProblemIDs[0], "language": "cpp", "source": strings.Repeat("x", (128<<10)+1)}
	f.request("POST", "/api/submissions", body, cookie, 400)
	if err := f.store.Update(func(s *model.State) error {
		for i := range s.Problems {
			if s.Problems[i].ID == work.ProblemIDs[0] {
				s.Problems[i].Samples = nil
				s.Problems[i].Tests = nil
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	body["source"] = "int main() {}"
	f.request("POST", "/api/submissions", body, cookie, 400)
	if len(f.store.Snapshot().Submissions) != 0 {
		t.Fatal("rejected submission was persisted")
	}
	select {
	case <-runner.calls:
		t.Fatal("rejected submission reached executor")
	default:
	}
}

func TestInconsistentRunnerSuccessBecomesInfrastructureError(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("badresult")
	sub := sendSolution(f, cookie, "c")
	_ = takeJob(t, runner)
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: 0, TotalTests: 3}}
	_ = awaitSubmission(t, f.store, sub.ID, "judge_error")
}
