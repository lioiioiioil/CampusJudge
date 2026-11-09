package app

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) createGroup(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 2 || len(in.Name) > 100 {
		return fail(400, "Название группы: от 2 до 100 символов")
	}
	g := model.Group{ID: newID("group"), Name: in.Name, InviteCode: strings.ToUpper(newID("join")[5:15])}
	if err := a.store.Update(func(s *model.State) error { s.Groups = append(s.Groups, g); return nil }); err != nil {
		return err
	}
	writeJSON(w, 201, g)
	return nil
}
func (a *App) createTopic(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		LessonURL   string `json:"lesson_url"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Title = strings.TrimSpace(in.Title)
	in.LessonURL = strings.TrimSpace(in.LessonURL)
	if len(in.Title) < 2 || len(in.Title) > 200 || len(in.Description) > 10000 {
		return fail(400, "Укажите название темы до 200 символов и описание до 10000 символов")
	}
	if in.LessonURL != "" {
		u, err := url.Parse(in.LessonURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(in.LessonURL) > 2000 {
			return fail(400, "Ссылка на занятие должна начинаться с https://")
		}
	}
	topic := model.Topic{ID: newID("topic"), Title: in.Title, Description: in.Description, LessonURL: in.LessonURL}
	if err := a.store.Update(func(s *model.State) error { s.Topics = append(s.Topics, topic); return nil }); err != nil {
		return err
	}
	writeJSON(w, 201, topic)
	return nil
}
func (a *App) createProblem(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in model.Problem
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if len(in.TestRevisions) != 0 {
		return fail(400, "Добавляйте версии тестов к сохранённой задаче")
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Statement = strings.TrimSpace(in.Statement)
	if len(in.Title) < 2 || len(in.Title) > 200 || len(in.Statement) < 5 || len(in.Statement) > 50000 {
		return fail(400, "Укажите название задачи и условие от 5 до 50000 символов")
	}
	if len(in.InputFormat) > 10000 || len(in.OutputFormat) > 10000 {
		return fail(400, "Слишком длинное описание формата данных")
	}
	if in.TimeLimitMS == 0 {
		in.TimeLimitMS = 2000
	}
	if in.MemoryLimitMB == 0 {
		in.MemoryLimitMB = 256
	}
	if in.TimeLimitMS < 100 || in.TimeLimitMS > 30000 || in.MemoryLimitMB < 16 || in.MemoryLimitMB > 2048 {
		return fail(400, "Лимит времени: 100–30000 мс; памяти: 16–2048 МБ")
	}
	if err := validateTestCases(in.Samples, in.Tests); err != nil {
		return err
	}
	in.ID = newID("problem")
	if in.Samples == nil {
		in.Samples = []model.TestCase{}
	}
	if in.Tests == nil {
		in.Tests = []model.TestCase{}
	}
	if err := a.store.Update(func(s *model.State) error { s.Problems = append(s.Problems, in); return nil }); err != nil {
		return err
	}
	writeJSON(w, 201, in)
	return nil
}

func validateTestCases(samples, hidden []model.TestCase) error {
	if len(samples) > 20 || len(hidden) > 100 {
		return fail(400, "Допускается до 20 примеров и до 100 скрытых тестов")
	}
	for _, tests := range [][]model.TestCase{samples, hidden} {
		for _, t := range tests {
			if len(t.Input) > 65536 || len(t.Output) > 65536 {
				return fail(400, "Размер одного теста ограничен 64 КБ")
			}
		}
	}
	return nil
}

type workDetails struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	TopicID     string     `json:"topic_id"`
	GroupIDs    []string   `json:"group_ids"`
	ProblemIDs  []string   `json:"problem_ids"`
	StartsAt    *time.Time `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
}

func (in workDetails) apply(work model.Work) model.Work {
	work.Title = strings.TrimSpace(in.Title)
	work.Description = in.Description
	work.TopicID = in.TopicID
	work.GroupIDs = append([]string{}, in.GroupIDs...)
	work.ProblemIDs = append([]string{}, in.ProblemIDs...)
	work.ProblemRevisions = maps.Clone(work.ProblemRevisions)
	maps.DeleteFunc(work.ProblemRevisions, func(id string, _ int) bool { return !slices.Contains(work.ProblemIDs, id) })
	work.StartsAt = in.StartsAt
	work.EndsAt = in.EndsAt
	return work
}

func (a *App) createWork(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in struct {
		workDetails
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	work := in.apply(model.Work{ID: newID("work"), Kind: in.Kind, Status: in.Status, CreatedAt: a.now()})
	if err := a.store.Update(func(s *model.State) error {
		if err := domain.ValidateWork(*s, work); err != nil {
			return fail(http.StatusBadRequest, err.Error())
		}
		s.Works = append(s.Works, work)
		return nil
	}); err != nil {
		return err
	}
	writeJSON(w, 201, work)
	return nil
}

func (a *App) editWorkDraft(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in workDetails
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	var result model.Work
	if err := a.store.Update(func(s *model.State) error {
		for i, work := range s.Works {
			if work.ID != r.PathValue("id") {
				continue
			}
			if !domain.CanEditDraft(work, s.Submissions) {
				return fail(http.StatusConflict, "Редактировать можно только черновик без посылок")
			}
			updated := in.apply(work)
			if err := domain.ValidateWork(*s, updated); err != nil {
				return fail(http.StatusBadRequest, err.Error())
			}
			s.Works[i] = updated
			result = updated
			return nil
		}
		return fail(http.StatusNotFound, "Работа не найдена")
	}); err != nil {
		return err
	}
	writeJSON(w, 200, result)
	return nil
}

func (a *App) updateWork(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in struct {
		Status string `json:"status"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	if !domain.ValidWorkStatus(in.Status) {
		return fail(400, "Неизвестный статус работы")
	}
	var result model.Work
	if err := a.store.Update(func(s *model.State) error {
		for i := range s.Works {
			if s.Works[i].ID == r.PathValue("id") {
				s.Works[i].Status = in.Status
				if err := domain.ValidateWork(*s, s.Works[i]); err != nil {
					return fail(http.StatusBadRequest, err.Error())
				}
				result = s.Works[i]
				return nil
			}
		}
		return fail(404, "Работа не найдена")
	}); err != nil {
		return err
	}
	writeJSON(w, 200, result)
	return nil
}
