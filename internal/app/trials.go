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

func (a *App) trial(w http.ResponseWriter, r *http.Request, user model.User) error {
	var in struct {
		auxiliarySource
		WorkID       string  `json:"work_id"`
		ProblemID    string  `json:"problem_id"`
		SampleIndex  *int    `json:"sample_index"`
		TestRevision *int    `json:"test_revision"`
		Input        *string `json:"input"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if err := in.auxiliarySource.validate(user); err != nil {
		return err
	}
	if (in.SampleIndex == nil) == (in.Input == nil) {
		return fail(http.StatusBadRequest, "Выберите один открытый пример или укажите свой ввод")
	}
	state := a.store.Snapshot()
	var work model.Work
	if index := slices.IndexFunc(state.Works, func(work model.Work) bool { return work.ID == in.WorkID }); index >= 0 {
		work = state.Works[index]
	}
	if err := domain.CanSubmit(work, user, in.ProblemID, a.now()); err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			return fail(http.StatusNotFound, err.Error())
		case errors.Is(err, domain.ErrClosed), errors.Is(err, domain.ErrStudentOnly):
			return fail(http.StatusForbidden, err.Error())
		default:
			return fail(http.StatusBadRequest, err.Error())
		}
	}
	problem := problemByID(state, in.ProblemID)
	if problem.ID == "" {
		return fail(http.StatusNotFound, "Задача не найдена")
	}
	var test model.TestCase
	if in.SampleIndex != nil {
		revision, found := domain.TestsAt(problem, work.ProblemRevisions[problem.ID])
		if !found || (in.TestRevision != nil && *in.TestRevision != revision.Number) {
			return fail(http.StatusConflict, "Версия тестов изменилась. Обновите данные и откройте задачу снова")
		}
		index := *in.SampleIndex
		if index < 0 || index >= len(revision.Samples) {
			return fail(http.StatusBadRequest, "Открытый пример не найден")
		}
		test = revision.Samples[index]
	} else {
		test.Input = *in.Input
		if len(test.Input) > protocol.MaxTestBytes {
			return fail(http.StatusBadRequest, "Размер ввода ограничен 64 КБ")
		}
	}
	executor, ok := a.runner.(judge.TrialRunner)
	if !ok {
		return fail(http.StatusServiceUnavailable, "Пробный запуск требует автоматического исполнителя")
	}
	job := judge.Job{Language: in.Language, Source: in.Source, Tests: []model.TestCase{test},
		TimeLimitMS: problem.TimeLimitMS, MemoryLimitMB: problem.MemoryLimitMB}
	result, err := a.startAuxiliary(user, state.Submissions, time.Minute, func(ctx context.Context) (auxiliaryResult, error) {
		trial, err := executor.Trial(ctx, job, in.SampleIndex != nil)
		return auxiliaryResult{Verdict: trial.Verdict, TimeMS: trial.TimeMS,
			Output:          trial.Output[:min(len(trial.Output), judge.MaxTrialOutputBytes)],
			OutputTruncated: trial.OutputTruncated || len(trial.Output) > judge.MaxTrialOutputBytes,
			CompileOutput:   trial.CompileOutput[:min(len(trial.CompileOutput), protocol.MaxCompilerLogBytes)]}, err
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, result)
	return nil
}
