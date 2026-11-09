package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type referenceTestRunner struct {
	*controlledRunner
	jobs chan judge.Job
}

func (runner *referenceTestRunner) Verify(ctx context.Context, job judge.Job) (judge.Result, error) {
	select {
	case runner.jobs <- job:
	case <-ctx.Done():
		return judge.Result{}, ctx.Err()
	}
	return judge.Result{Verdict: "wrong_answer", PassedTests: 2, TotalTests: 3, FailedTest: 2, TimeMS: 5}, nil
}

func TestReferenceCheckUsesSelectedRevisionAndKeepsDataPrivate(t *testing.T) {
	f := setup(t)
	teacher := f.store.Snapshot().Users[0]
	student, cookie := f.student("referencepupil")
	problem := f.store.Snapshot().Problems[0]
	samples := []model.TestCase{{Input: "reference v2 sample", Output: "5"}}
	tests := []model.TestCase{{Input: "reference v2 private one", Output: "6"},
		{Input: "reference v2 private two", Output: "7"}}
	f.request(http.MethodPost, "/api/problems/"+problem.ID+"/test-revisions", map[string][]model.TestCase{
		"samples": samples, "tests": tests,
	}, f.teacher, http.StatusCreated)
	path := "/api/problems/" + problem.ID + "/reference-check"
	body := map[string]any{"actor_id": teacher.ID, "source": "print(5) # private reference source",
		"language": "python", "test_revision": 2}
	f.request(http.MethodPost, path, body, nil, http.StatusUnauthorized)
	f.request(http.MethodPost, path, body, f.teacher, http.StatusServiceUnavailable)
	runner := &referenceTestRunner{newControlledRunner(), make(chan judge.Job, 1)}
	startAuxiliaryTest(t, f.app, runner)
	body["actor_id"] = student.ID
	f.request(http.MethodPost, path, body, cookie, http.StatusForbidden)
	body["actor_id"] = teacher.ID
	body["test_revision"] = 999
	f.request(http.MethodPost, path, body, f.teacher, http.StatusBadRequest)
	body["test_revision"] = 2
	admitted := decode[auxiliaryResult](t, f.request(http.MethodPost, path, body, f.teacher, http.StatusAccepted))
	f.request(http.MethodGet, "/api/auxiliary-runs/"+admitted.ID, nil, cookie, http.StatusNotFound)
	result := awaitAuxiliaryRun(t, f, f.teacher, admitted.ID)
	if result.Status != "done" || result.Verdict != "wrong_answer" || result.PassedTests != 2 ||
		result.TotalTests != 3 || result.FailedTest != 2 || result.TimeMS != 5 || result.Output != "" {
		t.Fatalf("unexpected report: %+v", result)
	}
	job := <-runner.jobs
	if len(job.Tests) != 3 || job.Tests[0] != samples[0] ||
		job.Tests[1] != tests[0] || job.Tests[2] != tests[1] {
		t.Fatal("reference job did not include examples and all hidden tests from revision 2 in order")
	}
	report := f.request(http.MethodGet, "/api/auxiliary-runs/"+admitted.ID, nil, f.teacher, http.StatusOK).Body.String()
	for _, private := range []string{tests[0].Input, tests[1].Input, "private reference source", `"output":`} {
		if strings.Contains(report, private) {
			t.Fatalf("reference report disclosed raw data: %q", private)
		}
	}
	if len(f.store.Snapshot().Submissions) != 0 {
		t.Fatal("reference check created a scored submission")
	}
}
