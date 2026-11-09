package app

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/judge/protocol"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) referenceCheck(w http.ResponseWriter, r *http.Request, user model.User) error {
	var in struct {
		auxiliarySource
		TestRevision int `json:"test_revision"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if err := in.auxiliarySource.validate(user); err != nil {
		return err
	}
	state := a.store.Snapshot()
	problem := problemByID(state, r.PathValue("id"))
	if problem.ID == "" {
		return fail(http.StatusNotFound, "Задача не найдена")
	}
	revision, found := domain.TestsAt(problem, in.TestRevision)
	if !found {
		return fail(http.StatusBadRequest, "Версия тестов не найдена")
	}
	tests := slices.Concat(revision.Samples, revision.Tests)
	if len(tests) == 0 {
		return fail(http.StatusBadRequest, "Добавьте хотя бы один тест перед проверкой эталона")
	}
	executor, ok := a.runner.(judge.ReferenceRunner)
	if !ok {
		return fail(http.StatusServiceUnavailable, "Проверка эталона требует автоматического исполнителя")
	}
	job := judge.Job{Language: in.Language, Source: in.Source, Tests: tests,
		TimeLimitMS: problem.TimeLimitMS, MemoryLimitMB: problem.MemoryLimitMB}
	result, err := a.startAuxiliary(user, state.Submissions, 90*time.Second, func(ctx context.Context) (auxiliaryResult, error) {
		check, err := executor.Verify(ctx, job)
		if err != nil {
			return auxiliaryResult{}, err
		}
		if check.TotalTests != len(tests) || check.PassedTests < 0 || check.PassedTests > check.TotalTests ||
			check.FailedTest < 0 || check.FailedTest > check.TotalTests || check.TimeMS < 0 ||
			(check.Verdict == "accepted" && (check.PassedTests != check.TotalTests || check.FailedTest != 0)) {
			return auxiliaryResult{}, errors.New("reference runner returned an inconsistent result")
		}
		return auxiliaryResult{Verdict: check.Verdict, TimeMS: check.TimeMS,
			PassedTests: check.PassedTests, TotalTests: check.TotalTests, FailedTest: check.FailedTest,
			CompileOutput: check.CompileOutput[:min(len(check.CompileOutput), protocol.MaxCompilerLogBytes)]}, nil
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, result)
	return nil
}
