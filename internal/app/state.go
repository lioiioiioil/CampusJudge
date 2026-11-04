package app

import (
	"net/http"
	"slices"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

var Languages = []map[string]string{{"id": "cpp", "name": "C++"}, {"id": "python", "name": "Python"}, {"id": "go", "name": "Go"}, {"id": "java", "name": "Java"}, {"id": "javascript", "name": "JavaScript"}, {"id": "c", "name": "C"}, {"id": "rust", "name": "Rust"}, {"id": "csharp", "name": "C#"}, {"id": "kotlin", "name": "Kotlin"}, {"id": "pascal", "name": "Pascal"}, {"id": "other", "name": "Другой язык"}}

func visibleWork(work model.Work, user model.User) bool {
	return user.Role == "teacher" || (work.Status != "draft" && slices.Contains(work.GroupIDs, user.GroupID))
}
func started(work model.Work, now time.Time) bool {
	return work.StartsAt == nil || !now.Before(*work.StartsAt)
}
func (a *App) state(w http.ResponseWriter, r *http.Request, user model.User) error {
	state := a.store.Snapshot()
	now := a.now()
	works := make([]model.Work, 0)
	problems := make([]model.Problem, 0)
	submissions := make([]model.Submission, 0)
	groups := make([]model.Group, 0)
	topics := make([]model.Topic, 0)
	users := make([]model.PublicUser, 0)
	allowedProblems := map[string]bool{}
	allowedWorks := map[string]bool{}
	for _, work := range state.Works {
		if visibleWork(work, user) {
			works = append(works, work)
			allowedWorks[work.ID] = true
			if user.Role == "teacher" || started(work, now) {
				for _, id := range work.ProblemIDs {
					allowedProblems[id] = true
				}
			}
		}
	}
	for _, p := range state.Problems {
		if user.Role == "teacher" || allowedProblems[p.ID] {
			if user.Role != "teacher" {
				p.Tests = nil
			}
			problems = append(problems, p)
		}
	}
	for _, s := range state.Submissions {
		if user.Role == "teacher" || (s.UserID == user.ID && allowedWorks[s.WorkID]) {
			submissions = append(submissions, s)
		}
	}
	for _, g := range state.Groups {
		if user.Role == "teacher" || g.ID == user.GroupID {
			if user.Role != "teacher" {
				g.InviteCode = ""
			}
			groups = append(groups, g)
		}
	}
	// Topics are shared course materials; publishing homework is independent.
	topics = append(topics, state.Topics...)
	for _, u := range state.Users {
		if user.Role == "teacher" || u.ID == user.ID {
			users = append(users, u.Public())
		}
	}
	writeJSON(w, 200, map[string]any{"user": user.Public(), "users": users, "groups": groups, "topics": topics, "problems": problems, "works": works, "submissions": submissions, "languages": Languages, "server_time": now, "judging_mode": "manual", "demo": state.Demo})
	return nil
}
