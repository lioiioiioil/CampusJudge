package app

import (
	"net/http"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type stateResponse struct {
	domain.Classroom
	Languages   []language `json:"languages"`
	ServerTime  time.Time  `json:"server_time"`
	JudgingMode string     `json:"judging_mode"`
	Demo        bool       `json:"demo"`
}

func (a *App) state(w http.ResponseWriter, r *http.Request, user model.User) error {
	snapshot := a.store.Snapshot()
	now := a.now()
	response := stateResponse{
		Classroom:   domain.ClassroomFor(snapshot, user, now),
		Languages:   languageCatalog(a.runner != nil),
		ServerTime:  now,
		JudgingMode: a.judgingMode(),
		Demo:        snapshot.Demo,
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}
