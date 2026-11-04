package app

import (
	"net/http"
	"net/url"
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
	if len(in.Samples) > 20 || len(in.Tests) > 100 {
		return fail(400, "Допускается до 20 примеров и до 100 скрытых тестов")
	}
	for _, tests := range [][]model.TestCase{in.Samples, in.Tests} {
		for _, t := range tests {
			if len(t.Input) > 65536 || len(t.Output) > 65536 {
				return fail(400, "Размер одного теста ограничен 64 КБ")
			}
		}
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
func (a *App) createWork(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Kind        string     `json:"kind"`
		TopicID     string     `json:"topic_id"`
		GroupIDs    []string   `json:"group_ids"`
		ProblemIDs  []string   `json:"problem_ids"`
		Status      string     `json:"status"`
		StartsAt    *time.Time `json:"starts_at"`
		EndsAt      *time.Time `json:"ends_at"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Status == "" {
		in.Status = "draft"
	}
	work := model.Work{
		ID:          newID("work"),
		Title:       in.Title,
		Description: in.Description,
		Kind:        in.Kind,
		TopicID:     in.TopicID,
		GroupIDs:    in.GroupIDs,
		ProblemIDs:  in.ProblemIDs,
		Status:      in.Status,
		StartsAt:    in.StartsAt,
		EndsAt:      in.EndsAt,
		CreatedAt:   a.now(),
	}
	if work.GroupIDs == nil {
		work.GroupIDs = []string{}
	}
	if work.ProblemIDs == nil {
		work.ProblemIDs = []string{}
	}
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
