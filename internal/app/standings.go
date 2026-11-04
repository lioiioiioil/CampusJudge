package app

import (
	"net/http"
	"slices"
	"sort"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type problemScore struct {
	Solved   bool `json:"solved"`
	Attempts int  `json:"attempts"`
	Minutes  int  `json:"minutes"`
}
type standingRow struct {
	UserID   string                   `json:"user_id"`
	Name     string                   `json:"name"`
	Solved   int                      `json:"solved"`
	Penalty  int                      `json:"penalty"`
	Problems map[string]*problemScore `json:"problems"`
}

func (a *App) standings(w http.ResponseWriter, r *http.Request, user model.User) error {
	state := a.store.Snapshot()
	var work model.Work
	for _, v := range state.Works {
		if v.ID == r.PathValue("id") {
			work = v
			break
		}
	}
	if work.ID == "" || !visibleWork(work, user) {
		return fail(404, "Работа не найдена")
	}
	if !started(work, a.now()) && user.Role != "teacher" {
		return fail(403, "Контест ещё не начался")
	}
	rows := make([]standingRow, 0)
	index := map[string]int{}
	for _, u := range state.Users {
		if u.Role == "student" && slices.Contains(work.GroupIDs, u.GroupID) {
			index[u.ID] = len(rows)
			rows = append(rows, standingRow{UserID: u.ID, Name: u.Name, Problems: make(map[string]*problemScore)})
		}
	}
	submissions := slices.Clone(state.Submissions)
	sort.SliceStable(submissions, func(i, j int) bool { return submissions[i].CreatedAt.Before(submissions[j].CreatedAt) })
	for _, sub := range submissions {
		if sub.WorkID != work.ID {
			continue
		}
		idx, ok := index[sub.UserID]
		if !ok {
			continue
		}
		row := &rows[idx]
		score := row.Problems[sub.ProblemID]
		if score == nil {
			score = &problemScore{}
			row.Problems[sub.ProblemID] = score
		}
		if score.Solved || sub.Verdict == "pending" {
			continue
		}
		if sub.Verdict == "accepted" {
			score.Solved = true
			row.Solved++
			if work.Kind == "contest" && work.StartsAt != nil {
				score.Minutes = max(0, int(sub.CreatedAt.Sub(*work.StartsAt).Minutes()))
				row.Penalty += score.Minutes + 20*score.Attempts
			}
		} else if sub.Verdict != "compilation_error" {
			score.Attempts++
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Solved != rows[j].Solved {
			return rows[i].Solved > rows[j].Solved
		}
		if rows[i].Penalty != rows[j].Penalty {
			return rows[i].Penalty < rows[j].Penalty
		}
		return rows[i].Name < rows[j].Name
	})
	writeJSON(w, 200, map[string]any{"rows": rows})
	return nil
}
