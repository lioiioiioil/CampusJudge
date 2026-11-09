package app

import (
	"net/http"
	"slices"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) createTestRevision(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var revision model.TestRevision
	var input struct {
		Samples []model.TestCase `json:"samples"`
		Tests   []model.TestCase `json:"tests"`
	}
	if err := readJSON(w, r, &input); err != nil {
		return err
	}
	if err := validateTestCases(input.Samples, input.Tests); err != nil {
		return err
	}
	if len(input.Samples)+len(input.Tests) == 0 {
		return fail(400, "Добавьте хотя бы один тест")
	}
	if err := a.store.Update(func(s *model.State) error {
		for i := range s.Problems {
			problem := &s.Problems[i]
			if problem.ID != r.PathValue("id") {
				continue
			}
			if len(problem.TestRevisions) >= 31 {
				return fail(409, "Для задачи сохранено 32 версии тестов")
			}
			revision = model.TestRevision{Number: len(problem.TestRevisions) + 2,
				Samples: input.Samples, Tests: input.Tests, CreatedAt: a.now()}
			problem.TestRevisions = append(problem.TestRevisions, revision)
			return nil
		}
		return fail(404, "Задача не найдена")
	}); err != nil {
		return err
	}
	writeJSON(w, 201, revision)
	return nil
}

func (a *App) setWorkTestRevision(w http.ResponseWriter, r *http.Request, _ model.User) error {
	var in struct {
		ProblemID string `json:"problem_id"`
		Revision  int    `json:"revision"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return err
	}
	var work model.Work
	if err := a.store.Update(func(s *model.State) error {
		for i := range s.Works {
			candidate := &s.Works[i]
			if candidate.ID != r.PathValue("id") {
				continue
			}
			if !slices.Contains(candidate.ProblemIDs, in.ProblemID) {
				return fail(400, "В этой работе нет выбранной задачи")
			}
			if _, ok := domain.TestsAt(problemByID(*s, in.ProblemID), in.Revision); in.Revision < 1 || !ok {
				return fail(400, "Версия тестов не найдена")
			}
			if in.Revision == 1 {
				delete(candidate.ProblemRevisions, in.ProblemID)
			} else {
				if candidate.ProblemRevisions == nil {
					candidate.ProblemRevisions = make(map[string]int)
				}
				candidate.ProblemRevisions[in.ProblemID] = in.Revision
			}
			work = *candidate
			return nil
		}
		return fail(404, "Работа не найдена")
	}); err != nil {
		return err
	}
	writeJSON(w, 200, work)
	return nil
}
