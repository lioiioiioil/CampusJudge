package app

import (
	"net/http"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type language struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// This catalog describes accepted source labels, not installed compilers.
var sourceLanguages = []language{
	{ID: "cpp", Name: "C++"},
	{ID: "python", Name: "Python"},
	{ID: "go", Name: "Go"},
	{ID: "java", Name: "Java"},
	{ID: "javascript", Name: "JavaScript"},
	{ID: "c", Name: "C"},
	{ID: "rust", Name: "Rust"},
	{ID: "csharp", Name: "C#"},
	{ID: "kotlin", Name: "Kotlin"},
	{ID: "pascal", Name: "Pascal"},
	{ID: "other", Name: "Другой язык"},
}

func languageSupported(id string) bool {
	for _, language := range sourceLanguages {
		if language.ID == id {
			return true
		}
	}
	return false
}

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
		Languages:   sourceLanguages,
		ServerTime:  now,
		JudgingMode: "manual",
		Demo:        snapshot.Demo,
	}
	writeJSON(w, http.StatusOK, response)
	return nil
}
