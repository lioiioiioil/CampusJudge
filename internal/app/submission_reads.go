package app

import (
	"net/http"
	"strings"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

const maxStatusIDs = 64

// The nil source shadows the embedded JSON field. Full responses and storage
// keep their established representation, including empty legacy source values.
type submissionSummary struct {
	model.Submission
	Source *string `json:"source,omitempty"`
}

func summarizeSubmissions(submissions []model.Submission) []submissionSummary {
	summaries := make([]submissionSummary, 0, len(submissions))
	for _, submission := range submissions {
		submission.Source = ""
		summaries = append(summaries, submissionSummary{Submission: submission})
	}
	return summaries
}

func (a *App) submissionDetails(w http.ResponseWriter, r *http.Request, user model.User) error {
	submissions := a.requestedSubmissions(user, map[string]bool{r.PathValue("id"): true})
	if len(submissions) == 0 {
		return fail(http.StatusNotFound, "Посылка не найдена")
	}
	writeJSON(w, http.StatusOK, struct {
		UserID     string           `json:"user_id"`
		Submission model.Submission `json:"submission"`
	}{user.ID, submissions[0]})
	return nil
}

func (a *App) submissionStatuses(w http.ResponseWriter, r *http.Request, user model.User) error {
	values := r.URL.Query()["ids"]
	if len(values) != 1 {
		return fail(http.StatusBadRequest, "Укажите один список идентификаторов посылок")
	}
	if strings.Count(values[0], ",") >= maxStatusIDs {
		return fail(http.StatusBadRequest, "Запрашивайте до 64 посылок за раз")
	}
	ids := make(map[string]bool)
	for id := range strings.SplitSeq(values[0], ",") {
		if id == "" || len(id) > 128 {
			return fail(http.StatusBadRequest, "Некорректный идентификатор посылки")
		}
		ids[id] = true
	}
	writeJSON(w, http.StatusOK, struct {
		UserID      string              `json:"user_id"`
		Submissions []submissionSummary `json:"submissions"`
	}{user.ID, summarizeSubmissions(a.requestedSubmissions(user, ids))})
	return nil
}

func (a *App) requestedSubmissions(user model.User, ids map[string]bool) []model.Submission {
	state := a.store.Snapshot()
	visibleWorks := make(map[string]bool)
	if user.Role != "teacher" {
		for _, work := range state.Works {
			visibleWorks[work.ID] = domain.WorkVisible(work, user)
		}
	}
	var submissions []model.Submission
	for _, submission := range state.Submissions {
		if ids[submission.ID] && domain.SubmissionVisible(submission, user, visibleWorks[submission.WorkID]) {
			submissions = append(submissions, submission)
		}
	}
	return submissions
}
