package domain

import (
	"slices"
	"sort"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type ProblemScore struct {
	Solved   bool `json:"solved"`
	Attempts int  `json:"attempts"`
	Minutes  int  `json:"minutes"`
}

type StandingRow struct {
	UserID   string                   `json:"user_id"`
	Name     string                   `json:"name"`
	Solved   int                      `json:"solved"`
	Penalty  int                      `json:"penalty"`
	Problems map[string]*ProblemScore `json:"problems"`
}

// ComputeStandings calculates scores from recorded verdicts. Attempts counts
// failed submissions before the first acceptance, excluding compilation errors
// and pending submissions. Each solved contest problem adds its acceptance
// minute plus 20 minutes per such failed attempt; homework has no time penalty.
// It does not authorize the viewer or mutate the supplied data.
func ComputeStandings(work model.Work, users []model.User, submissions []model.Submission) []StandingRow {
	rows := make([]StandingRow, 0)
	index := make(map[string]int)
	for _, user := range users {
		if user.Role == "student" && slices.Contains(work.GroupIDs, user.GroupID) {
			index[user.ID] = len(rows)
			rows = append(rows, StandingRow{UserID: user.ID, Name: user.Name, Problems: make(map[string]*ProblemScore)})
		}
	}

	ordered := slices.Clone(submissions)
	// Equal timestamps retain their persisted arrival order. A random ID is not
	// evidence that a failed submission arrived before an accepted one.
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].CreatedAt.Before(ordered[j].CreatedAt) })
	for _, submission := range ordered {
		if submission.WorkID != work.ID {
			continue
		}
		rowIndex, ok := index[submission.UserID]
		if !ok {
			continue
		}
		row := &rows[rowIndex]
		score := row.Problems[submission.ProblemID]
		if score == nil {
			score = &ProblemScore{}
			row.Problems[submission.ProblemID] = score
		}
		if score.Solved || submission.Verdict == "pending" {
			continue
		}
		if submission.Verdict == "accepted" {
			score.Solved = true
			row.Solved++
			if work.Kind == "contest" && work.StartsAt != nil {
				score.Minutes = max(0, int(submission.CreatedAt.Sub(*work.StartsAt).Minutes()))
				row.Penalty += score.Minutes + 20*score.Attempts
			}
		} else if submission.Verdict != "compilation_error" {
			score.Attempts++
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Solved != rows[j].Solved {
			return rows[i].Solved > rows[j].Solved
		}
		if rows[i].Penalty != rows[j].Penalty {
			return rows[i].Penalty < rows[j].Penalty
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].UserID < rows[j].UserID
	})
	return rows
}
