package app

import (
	"errors"
	"net/http"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestTeacherRetriesInfrastructureFailureWithoutCreatingAttempt(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("retry")
	sub := sendSolution(f, cookie, "python")
	job := takeJob(t, runner)
	runner.replies <- judgeReply{err: errors.New("Docker unavailable")}
	awaitSubmission(t, f.store, sub.ID, "judge_error")
	path := "/api/submissions/" + sub.ID + "/retry"
	f.request("POST", path, nil, cookie, http.StatusForbidden)
	queued := decode[model.Submission](t, f.request("POST", path, nil, f.teacher, http.StatusAccepted))
	if queued.ID != sub.ID || !queued.CreatedAt.Equal(sub.CreatedAt) || queued.TestRevision != sub.TestRevision {
		t.Fatal("retry changed the identity, arrival time or tests of the original attempt")
	}
	retried := takeJob(t, runner)
	if retried.Source != job.Source || len(retried.Tests) != len(job.Tests) || len(f.store.Snapshot().Submissions) != 1 {
		t.Fatal("retry duplicated the submission or ran different source/tests")
	}
	f.request("POST", path, nil, f.teacher, http.StatusConflict)
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: len(job.Tests), TotalTests: len(job.Tests)}}
	awaitSubmission(t, f.store, sub.ID, "accepted")
}
