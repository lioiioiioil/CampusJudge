package app

import (
	"net/http"
	"slices"
	"strings"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) submit(w http.ResponseWriter, r *http.Request, user model.User) error {
	if user.Role != "student" {
		return fail(403, "Отправка решений доступна из аккаунта ученика")
	}
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
	languageOK := false
	for _, l := range Languages {
		if l["id"] == in.Language {
			languageOK = true
		}
	}
	if !languageOK {
		return fail(400, "Выберите язык решения")
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
		if work.ID == "" || !visibleWork(work, user) {
			return fail(404, "Работа не найдена")
		}
		now := a.now()
		if work.Status != "open" || !started(work, now) || (work.EndsAt != nil && !now.Before(*work.EndsAt)) {
			return fail(403, "Приём решений закрыт или ещё не начался")
		}
		if !slices.Contains(work.ProblemIDs, in.ProblemID) {
			return fail(400, "В этой работе нет выбранной задачи")
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
