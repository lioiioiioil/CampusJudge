package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestTestRevisionPinsQueuedSubmissionsAndProtectsHiddenData(t *testing.T) {
	f := setup(t)
	runner := newControlledRunner()
	startControlled(t, f.app, runner)
	_, cookie := f.student("versions")
	work := f.store.Snapshot().Works[0]
	problemID := work.ProblemIDs[0]
	first := sendSolution(f, cookie, "python")
	firstJob := takeJob(t, runner)
	oldQueued := sendSolution(f, cookie, "python")
	path := "/api/problems/" + problemID + "/test-revisions"
	newTests := map[string][]model.TestCase{
		"samples": {{Input: "new-public-example", Output: "42"}},
		"tests":   {{Input: "new-hidden-input", Output: "hidden-expected-answer"}},
	}
	f.request("POST", path, newTests, cookie, http.StatusForbidden)
	revision := decode[model.TestRevision](t, f.request("POST", path, newTests, f.teacher, http.StatusCreated))
	if revision.Number != 2 || first.TestRevision != 1 || oldQueued.TestRevision != 1 {
		t.Fatalf("unexpected revision binding: new=%d, first=%d, queued=%d", revision.Number, first.TestRevision, oldQueued.TestRevision)
	}
	state := f.request("GET", "/api/state", nil, cookie, http.StatusOK).Body.String()
	if strings.Contains(state, "new-public-example") || strings.Contains(state, "new-hidden-input") {
		t.Fatal("unpublished revision was returned to a student")
	}
	activation := "/api/works/" + work.ID + "/problem-revisions"
	f.request("PATCH", activation, map[string]any{"problem_id": problemID, "revision": 3}, f.teacher, http.StatusBadRequest)
	f.request("PATCH", activation, map[string]any{"problem_id": problemID, "revision": 2}, f.teacher, http.StatusOK)
	state = f.request("GET", "/api/state", nil, cookie, http.StatusOK).Body.String()
	if !strings.Contains(state, "new-public-example") || strings.Contains(state, "new-hidden-input") || strings.Contains(state, "hidden-expected-answer") {
		t.Fatal("active public example or hidden test visibility is wrong")
	}
	newSubmission := sendSolution(f, cookie, "python")
	if newSubmission.TestRevision != 2 {
		t.Fatal("new submission did not bind to the activated revision")
	}
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: len(firstJob.Tests), TotalTests: len(firstJob.Tests)}}
	oldJob := takeJob(t, runner)
	if len(oldJob.Tests) != 3 || oldJob.Tests[0] != firstJob.Tests[0] {
		t.Fatalf("queued submission moved to a new revision: %+v", oldJob.Tests)
	}
	runner.replies <- judgeReply{result: judge.Result{Verdict: "accepted", PassedTests: 3, TotalTests: 3}}
	newJob := takeJob(t, runner)
	if len(newJob.Tests) != 2 || newJob.Tests[1].Input != "new-hidden-input" {
		t.Fatalf("wrong suite in new submission: %+v", newJob.Tests)
	}
}
