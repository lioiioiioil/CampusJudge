package app

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestRejudgePreservesEffectiveVerdictAcrossFailureAndRetry(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("rejudge")
	sub := sendSolution(f, cookie, "python")
	first := takeJob(t, runner)
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: len(first.Tests), TotalTests: len(first.Tests)}}
	initial := awaitSubmission(t, f.store, sub.ID, "accepted")
	f.request("POST", "/api/problems/"+sub.ProblemID+"/test-revisions", map[string][]model.TestCase{
		"tests": {{Input: "updated suite", Output: "changed"}},
	}, f.teacher, http.StatusCreated)
	path := "/api/submissions/" + sub.ID
	queued := decode[model.Submission](t, f.request("POST", path+"/rejudge", map[string]int{"revision": 2}, f.teacher, http.StatusAccepted))
	if queued.Verdict != "accepted" || queued.TestRevision != 1 || queued.Judging.TargetRevision != 2 || !queued.CreatedAt.Equal(initial.CreatedAt) {
		t.Fatalf("rejudge altered the effective grade or arrival time: %+v", queued)
	}
	job := takeJob(t, runner)
	if len(job.Tests) != 1 || job.Tests[0].Input != "updated suite" {
		t.Fatalf("rejudge ran the wrong test suite: %+v", job.Tests)
	}
	runner.replies <- judgeReply{err: errors.New("executor unavailable")}
	deadline := time.Now().Add(4 * time.Second)
	for {
		saved := f.store.Snapshot().Submissions[0]
		if saved.Judging.Status == "error" {
			if saved.Verdict != "accepted" || saved.TestRevision != 1 || len(saved.PastJudgments) != 0 {
				t.Fatalf("failed rejudge changed the effective result: %+v", saved)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("failed rejudge was not recorded")
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.request("POST", path+"/retry", nil, f.teacher, http.StatusAccepted)
	if retried := takeJob(t, runner); len(retried.Tests) != 1 || retried.Tests[0] != job.Tests[0] {
		t.Fatal("retry did not retain the target revision")
	}
	runner.replies <- judgeReply{result: judge.Result{Verdict: "wrong_answer", TotalTests: 1, FailedTest: 1}}
	result := awaitSubmission(t, f.store, sub.ID, "wrong_answer")
	if result.TestRevision != 2 || !result.CreatedAt.Equal(initial.CreatedAt) || len(result.PastJudgments) != 1 ||
		result.PastJudgments[0].Verdict != "accepted" || result.PastJudgments[0].TestRevision != 1 {
		t.Fatalf("new grade or history is wrong: %+v", result)
	}
	f.request("PATCH", path, map[string]string{"verdict": "accepted", "feedback": "Ручная проверка"}, f.teacher, http.StatusOK)
	f.request("POST", path+"/rejudge", map[string]int{"revision": 2}, f.teacher, http.StatusConflict)
}
