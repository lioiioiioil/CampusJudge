package app

import (
	"net/http"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) retryJudging(w http.ResponseWriter, r *http.Request, _ model.User) error {
	if a.runner == nil {
		return fail(409, "Автоматическая проверка выключена")
	}
	var result model.Submission
	if err := a.store.Update(func(s *model.State) error {
		for i := range s.Submissions {
			sub := &s.Submissions[i]
			if sub.ID != r.PathValue("id") {
				continue
			}
			if sub.ReviewerID != "" || sub.Verdict != "judge_error" || sub.Judging == nil || sub.Judging.Status != "error" {
				return fail(409, "Повтор доступен после сбоя автоматической проверки")
			}
			if err := enqueueJudgment(s, sub, sub.TestRevision); err != nil {
				return err
			}
			result = *sub
			return nil
		}
		return fail(404, "Посылка не найдена")
	}); err != nil {
		return err
	}
	a.wakeJudge()
	writeJSON(w, http.StatusAccepted, result)
	return nil
}

func enqueueJudgment(state *model.State, submission *model.Submission, revision int) error {
	tests, found := domain.TestsAt(problemByID(*state, submission.ProblemID), revision)
	total := len(tests.Samples) + len(tests.Tests)
	if !found || total == 0 || !automaticLanguage(submission.Language) {
		return fail(409, "Для этой посылки недоступна автоматическая проверка")
	}
	if err := checkJudgeCapacity(state.Submissions, submission.UserID); err != nil {
		return err
	}
	submission.Verdict = "pending"
	submission.JudgedAt = nil
	submission.TestRevision = tests.Number
	submission.Judging = &model.Judging{Status: "queued", TotalTests: total}
	return nil
}
