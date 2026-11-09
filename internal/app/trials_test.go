package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type trialCall struct {
	job     judge.Job
	compare bool
}

type auxiliaryTestRunner struct {
	*controlledRunner
	trials       chan trialCall
	trialResults chan judge.TrialResult
}

func (runner *auxiliaryTestRunner) Trial(ctx context.Context, job judge.Job, compare bool) (judge.TrialResult, error) {
	select {
	case runner.trials <- trialCall{job, compare}:
	case <-ctx.Done():
		return judge.TrialResult{}, ctx.Err()
	}
	select {
	case result := <-runner.trialResults:
		return result, nil
	case <-ctx.Done():
		return judge.TrialResult{}, ctx.Err()
	}
}

func startAuxiliaryTest(t *testing.T, app *App, runner judge.Runner) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if err := app.StartJudge(ctx, runner); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); app.WaitJudge() })
	return cancel
}

func trialRunnerFor(t *testing.T, app *App) *auxiliaryTestRunner {
	t.Helper()
	runner := &auxiliaryTestRunner{newControlledRunner(), make(chan trialCall, 1), make(chan judge.TrialResult, 1)}
	startAuxiliaryTest(t, app, runner)
	return runner
}

func awaitAuxiliaryRun(t *testing.T, f *fixture, cookie *http.Cookie, id string) auxiliaryResult {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		result := decode[auxiliaryResult](t, f.request(http.MethodGet, "/api/auxiliary-runs/"+id, nil, cookie, http.StatusOK))
		if result.Status != "running" {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("auxiliary run %q did not finish", id)
		}
		time.Sleep(time.Millisecond)
	}
}

func awaitTrialCall(t *testing.T, runner *auxiliaryTestRunner) trialCall {
	t.Helper()
	select {
	case call := <-runner.trials:
		return call
	case <-time.After(3 * time.Second):
		t.Fatal("trial did not reach executor")
		return trialCall{}
	}
}

func TestTrialPublicCustomAndOwnerBoundaries(t *testing.T) {
	f := setup(t)
	student, cookie := f.student("trialowner")
	other, otherCookie := f.student("trialother")
	runner := trialRunnerFor(t, f.app)
	work := f.store.Snapshot().Works[0]
	body := map[string]any{"actor_id": student.ID, "work_id": work.ID, "problem_id": work.ProblemIDs[0],
		"language": "python", "source": "print(5)", "sample_index": 1}
	// Only a single public sample exists. A sample index cannot reach hidden tests.
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusBadRequest)
	body["sample_index"] = 0
	f.request(http.MethodPost, "/api/trials", body, otherCookie, http.StatusConflict)
	body["input"] = ""
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusBadRequest)
	delete(body, "input")
	queued := decode[auxiliaryResult](t, f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusAccepted))
	if queued.ID == "" || queued.Status != "running" {
		t.Fatalf("unexpected admission: %+v", queued)
	}
	path := "/api/auxiliary-runs/" + queued.ID
	f.request(http.MethodGet, path, nil, nil, http.StatusUnauthorized)
	f.request(http.MethodGet, path, nil, otherCookie, http.StatusNotFound)
	call := awaitTrialCall(t, runner)
	if !call.compare || len(call.job.Tests) != 1 || call.job.Tests[0].Input != "2 3\n" || call.job.Tests[0].Output != "5\n" {
		t.Fatalf("trial inputs = %+v", call)
	}
	runner.trialResults <- judge.TrialResult{Verdict: "accepted", Output: strings.Repeat("x", judge.MaxTrialOutputBytes+1), TimeMS: 4}
	result := awaitAuxiliaryRun(t, f, cookie, queued.ID)
	if result.Verdict != "accepted" || result.TimeMS != 4 || len(result.Output) != judge.MaxTrialOutputBytes || !result.OutputTruncated {
		t.Fatalf("incorrect trial result: verdict=%s time=%d length=%d truncated=%t", result.Verdict, result.TimeMS, len(result.Output), result.OutputTruncated)
	}
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusTooManyRequests)
	body["actor_id"] = other.ID
	body["input"] = "  4  2\n"
	delete(body, "sample_index")
	custom := decode[auxiliaryResult](t, f.request(http.MethodPost, "/api/trials", body, otherCookie, http.StatusAccepted))
	call = awaitTrialCall(t, runner)
	if call.compare || len(call.job.Tests) != 1 || call.job.Tests[0].Input != "  4  2\n" || call.job.Tests[0].Output != "" {
		t.Fatalf("custom input changed: %+v", call)
	}
	runner.trialResults <- judge.TrialResult{Verdict: "completed", Output: "6\n"}
	if result := awaitAuxiliaryRun(t, f, otherCookie, custom.ID); result.Verdict != "completed" || result.Output != "6\n" {
		t.Fatalf("custom result: %+v", result)
	}
	if len(f.store.Snapshot().Submissions) != 0 {
		t.Fatal("trial created a scored submission")
	}
}

func TestTrialUsesActiveWorkRevisionAndRejectsStaleSample(t *testing.T) {
	f := setup(t)
	student, cookie := f.student("trialrevision")
	work := f.store.Snapshot().Works[0]
	problemID := work.ProblemIDs[0]
	samples := []model.TestCase{{Input: "new example\n", Output: "42\n"}}
	f.request(http.MethodPost, "/api/problems/"+problemID+"/test-revisions", map[string][]model.TestCase{
		"samples": samples,
		"tests":   {{Input: "private test", Output: "private answer"}},
	}, f.teacher, http.StatusCreated)
	f.request(http.MethodPatch, "/api/works/"+work.ID+"/problem-revisions",
		map[string]any{"problem_id": problemID, "revision": 2}, f.teacher, http.StatusOK)
	runner := trialRunnerFor(t, f.app)

	body := map[string]any{"actor_id": student.ID, "work_id": work.ID, "problem_id": problemID,
		"language": "python", "source": "print(42)", "sample_index": 0, "test_revision": 1}
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusConflict)
	body["test_revision"], body["sample_index"] = 2, 1
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusBadRequest)
	body["sample_index"] = 0
	run := decode[auxiliaryResult](t, f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusAccepted))
	call := awaitTrialCall(t, runner)
	if !call.compare || len(call.job.Tests) != 1 || call.job.Tests[0] != samples[0] {
		t.Fatalf("trial did not use the active public example: %+v", call)
	}
	runner.trialResults <- judge.TrialResult{Verdict: "accepted"}
	_ = awaitAuxiliaryRun(t, f, cookie, run.ID)
}

func TestTrialAdmissionAndSharedExecutionSlot(t *testing.T) {
	f := setup(t)
	student, cookie := f.student("trialstudent")
	other, otherCookie := f.student("trialclassmate")
	work := f.store.Snapshot().Works[0]
	body := map[string]any{"actor_id": student.ID, "work_id": work.ID, "problem_id": work.ProblemIDs[0],
		"language": "python", "source": "print(5)", "input": ""}
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusServiceUnavailable)
	runner := trialRunnerFor(t, f.app)
	body["work_id"] = "not-visible"
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusNotFound)
	body["work_id"] = work.ID
	body["input"] = strings.Repeat("я", 32769)
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusBadRequest)
	body["input"] = ""
	f.request(http.MethodPatch, "/api/works/"+work.ID, map[string]string{"status": "closed"}, f.teacher, http.StatusOK)
	f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusForbidden)
	f.request(http.MethodPatch, "/api/works/"+work.ID, map[string]string{"status": "open"}, f.teacher, http.StatusOK)
	trial := decode[auxiliaryResult](t, f.request(http.MethodPost, "/api/trials", body, cookie, http.StatusAccepted))
	_ = awaitTrialCall(t, runner)
	body["actor_id"] = other.ID
	f.request(http.MethodPost, "/api/trials", body, otherCookie, http.StatusTooManyRequests)

	submission := sendSolution(f, cookie, "python")
	deadline := time.Now().Add(3 * time.Second)
	for {
		snapshot := f.store.Snapshot()
		if snapshot.Submissions[0].Judging.Status == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("graded worker did not claim queued submission")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-runner.calls:
		t.Fatal("graded and trial jobs overlapped")
	default:
	}
	runner.trialResults <- judge.TrialResult{Verdict: "completed"}
	_ = awaitAuxiliaryRun(t, f, cookie, trial.ID)
	job := takeJob(t, runner.controlledRunner)
	f.request(http.MethodPost, "/api/trials", body, otherCookie, http.StatusTooManyRequests)
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: len(job.Tests), TotalTests: len(job.Tests)}}
	_ = awaitSubmission(t, f.store, submission.ID, "accepted")
}

func TestAuxiliaryResultsAreBoundedAndExpire(t *testing.T) {
	f := setup(t)
	now := time.Now()
	f.app.now = func() time.Time { return now }
	f.app.auxiliary = newAuxiliaryRuns(context.Background())
	for i := 0; i < auxiliaryResultLimit; i++ {
		id := newID("test")
		f.app.auxiliary.results[id] = cachedAuxiliaryRun{
			ownerID: "older", startedAt: now.Add(-time.Minute), expiresAt: now.Add(time.Minute),
			result: auxiliaryResult{ID: id, Status: "done"},
		}
	}
	_, err := f.app.startAuxiliary(model.User{ID: "next"}, nil, time.Second, func(context.Context) (auxiliaryResult, error) {
		return auxiliaryResult{Verdict: "completed"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.app.auxiliary.wait()
	if len(f.app.auxiliary.results) != auxiliaryResultLimit {
		t.Fatalf("result count = %d", len(f.app.auxiliary.results))
	}
	teacher := f.store.Snapshot().Users[0]
	f.app.auxiliary.results["expired"] = cachedAuxiliaryRun{
		ownerID: teacher.ID, expiresAt: now, result: auxiliaryResult{ID: "expired", Status: "done"},
	}
	f.request(http.MethodGet, "/api/auxiliary-runs/expired", nil, f.teacher, http.StatusNotFound)
}
