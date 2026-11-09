package app

import (
	"net/http"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) rejudgeSubmission(w http.ResponseWriter, r *http.Request, _ model.User) error {
	if a.runner == nil {
		return fail(409, "Автоматическая проверка выключена")
	}
	var in struct {
		Revision int `json:"revision"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Revision < 1 {
		return fail(400, "Выберите версию тестов")
	}
	var result model.Submission
	if err := a.store.Update(func(s *model.State) error {
		for i := range s.Submissions {
			sub := &s.Submissions[i]
			if sub.ID != r.PathValue("id") {
				continue
			}
			if sub.ReviewerID != "" || sub.Judging == nil || automaticPending(*sub) || !hasEffectiveVerdict(*sub) {
				return fail(409, "Перепроверка доступна для завершённого автоматического результата")
			}
			if err := enqueueJudgment(s, sub, in.Revision); err != nil {
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

func targetTestRevision(submission model.Submission) int {
	if submission.Judging != nil && submission.Judging.TargetRevision > 0 {
		return submission.Judging.TargetRevision
	}
	return max(1, submission.TestRevision)
}

func hasEffectiveVerdict(submission model.Submission) bool {
	return submission.Verdict != "" && submission.Verdict != "pending" && submission.Verdict != "judge_error"
}

func archiveJudgment(submission *model.Submission, at time.Time) {
	if hasEffectiveVerdict(*submission) {
		submission.PastJudgments = append(submission.PastJudgments, model.PastJudgment{
			Verdict: submission.Verdict, TestRevision: max(1, submission.TestRevision), ReplacedAt: at,
		})
	}
}
