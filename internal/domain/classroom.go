package domain

import (
	"maps"
	"slices"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type Classroom struct {
	User        model.PublicUser   `json:"user"`
	Users       []model.PublicUser `json:"users"`
	Groups      []model.Group      `json:"groups"`
	Topics      []model.Topic      `json:"topics"`
	Problems    []model.Problem    `json:"problems"`
	Works       []model.Work       `json:"works"`
	Submissions []model.Submission `json:"submissions"`
}

// ClassroomFor projects a stored classroom into the information this viewer
// may read. Topics are shared course materials; assignment publication controls
// works and their problems independently. Returned data does not alias state.
func ClassroomFor(state model.State, user model.User, now time.Time) Classroom {
	classroom := Classroom{
		User: user.Public(), Users: []model.PublicUser{}, Groups: []model.Group{},
		Topics: []model.Topic{}, Problems: []model.Problem{}, Works: []model.Work{},
		Submissions: []model.Submission{},
	}
	teacher := user.Role == "teacher"
	allowedWorks := make(map[string]bool)
	allowedRevisions := make(map[string]map[int]bool)
	for _, work := range state.Works {
		if !WorkVisible(work, user) {
			continue
		}
		allowedWorks[work.ID] = true
		if teacher || WorkStarted(work, now) {
			for _, id := range work.ProblemIDs {
				if allowedRevisions[id] == nil {
					allowedRevisions[id] = make(map[int]bool)
				}
				allowedRevisions[id][max(1, work.ProblemRevisions[id])] = true
			}
		}
		work.GroupIDs = slices.Clone(work.GroupIDs)
		work.ProblemIDs = slices.Clone(work.ProblemIDs)
		work.ProblemRevisions = maps.Clone(work.ProblemRevisions)
		work.StartsAt, work.EndsAt = copyTimestamp(work.StartsAt), copyTimestamp(work.EndsAt)
		classroom.Works = append(classroom.Works, work)
	}
	for _, problem := range state.Problems {
		revisions := allowedRevisions[problem.ID]
		if !teacher && len(revisions) == 0 {
			continue
		}
		classroom.Problems = append(classroom.Problems, projectProblem(problem, teacher, revisions))
	}
	for _, submission := range state.Submissions {
		if SubmissionVisible(submission, user, allowedWorks[submission.WorkID]) {
			submission.JudgedAt = copyTimestamp(submission.JudgedAt)
			submission.PastJudgments = slices.Clone(submission.PastJudgments)
			if submission.Judging != nil {
				progress := *submission.Judging
				submission.Judging = &progress
			}
			classroom.Submissions = append(classroom.Submissions, submission)
		}
	}
	for _, group := range state.Groups {
		if teacher || group.ID == user.GroupID {
			if !teacher {
				group.InviteCode = ""
			}
			classroom.Groups = append(classroom.Groups, group)
		}
	}
	classroom.Topics = append(classroom.Topics, state.Topics...)
	for _, candidate := range state.Users {
		if teacher || candidate.ID == user.ID {
			classroom.Users = append(classroom.Users, candidate.Public())
		}
	}
	return classroom
}

func projectProblem(problem model.Problem, teacher bool, visible map[int]bool) model.Problem {
	if teacher || visible[1] {
		problem.Samples = slices.Clone(problem.Samples)
	} else {
		problem.Samples = nil
	}
	if teacher {
		problem.Tests = slices.Clone(problem.Tests)
	} else {
		problem.Tests = nil
	}
	revisions := problem.TestRevisions
	problem.TestRevisions = nil
	for _, revision := range revisions {
		if !teacher && !visible[revision.Number] {
			continue
		}
		revision.Samples = slices.Clone(revision.Samples)
		if teacher {
			revision.Tests = slices.Clone(revision.Tests)
		} else {
			revision.Tests = nil
		}
		problem.TestRevisions = append(problem.TestRevisions, revision)
	}
	return problem
}

// SubmissionVisible applies the same access policy to classroom and direct reads.
// Callers supply work visibility computed for this viewer.
func SubmissionVisible(submission model.Submission, user model.User, workVisible bool) bool {
	return user.Role == "teacher" || (submission.UserID == user.ID && workVisible)
}

func copyTimestamp(timestamp *time.Time) *time.Time {
	if timestamp == nil {
		return nil
	}
	copy := *timestamp
	return &copy
}
