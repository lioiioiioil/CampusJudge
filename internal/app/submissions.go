package app

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) submit(w http.ResponseWriter, r *http.Request, user model.User) error {
	var in struct {
		WorkID    string `json:"work_id"`
		ProblemID string `json:"problem_id"`
		Language  string `json:"language"`
		Source    string `json:"source"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if len(strings.TrimSpace(in.Source)) == 0 || len(in.Source) > 256<<10 {
		return fail(400, "Отправьте исходный код размером до 256 КБ")
	}
	if !languageSupported(in.Language) {
		return fail(http.StatusBadRequest, "Выберите язык решения")
	}
	submission := model.Submission{ID: newID("submission"), WorkID: in.WorkID, ProblemID: in.ProblemID, UserID: user.ID, Language: in.Language, Source: in.Source, Verdict: "pending", CreatedAt: a.now()}
	if err := a.store.Update(func(s *model.State) error {
		var work model.Work
		for _, candidate := range s.Works {
			if candidate.ID == in.WorkID {
				work = candidate
				break
			}
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
		s.Submissions = append(s.Submissions, submission)
		return nil
	}); err != nil {
		return err
	}
	writeJSON(w, 201, submission)
	return nil
}
func (a *App) review(w http.ResponseWriter, r *http.Request, user model.User) error {
	var in struct {
		Verdict  string `json:"verdict"`
		Feedback string `json:"feedback"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if !slices.Contains([]string{"accepted", "wrong_answer", "compilation_error", "runtime_error", "time_limit", "memory_limit"}, in.Verdict) {
		return fail(400, "Выберите результат проверки")
	}
	if len(in.Feedback) > 10000 {
		return fail(400, "Комментарий ограничен 10000 символами")
	}
	var result model.Submission
	if err := a.store.Update(func(s *model.State) error {
		for i := range s.Submissions {
			if s.Submissions[i].ID == r.PathValue("id") {
				sub := &s.Submissions[i]
				sub.Verdict = in.Verdict
				sub.Feedback = in.Feedback
				now := a.now()
				sub.JudgedAt = &now
				sub.ReviewerID = user.ID
				result = *sub
				return nil
			}
		}
		return fail(404, "Посылка не найдена")
	}); err != nil {
		return err
	}
	writeJSON(w, 200, result)
	return nil
}
