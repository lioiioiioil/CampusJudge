package app

import (
	"net/http"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func (a *App) standings(w http.ResponseWriter, r *http.Request, user model.User) error {
	snapshot := a.store.Snapshot()
	var work model.Work
	for _, candidate := range snapshot.Works {
		if candidate.ID == r.PathValue("id") {
			work = candidate
			break
		}
	}
	if work.ID == "" || !domain.WorkVisible(work, user) {
		return fail(http.StatusNotFound, "Работа не найдена")
	}
	if user.Role != "teacher" && !domain.WorkStarted(work, a.now()) {
		return fail(http.StatusForbidden, "Контест ещё не начался")
	}
	response := struct {
		Rows []domain.StandingRow `json:"rows"`
	}{
		Rows: domain.ComputeStandings(work, snapshot.Users, snapshot.Submissions),
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}
