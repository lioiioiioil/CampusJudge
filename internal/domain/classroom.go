package domain

import (
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
	allowedProblems := make(map[string]bool)
	for _, work := range state.Works {
		if !WorkVisible(work, user) {
			continue
		}
		allowedWorks[work.ID] = true
		if teacher || WorkStarted(work, now) {
			for _, id := range work.ProblemIDs {
				allowedProblems[id] = true
			}
		}
		work.GroupIDs = slices.Clone(work.GroupIDs)
		work.ProblemIDs = slices.Clone(work.ProblemIDs)
		work.StartsAt, work.EndsAt = copyTimestamp(work.StartsAt), copyTimestamp(work.EndsAt)
		classroom.Works = append(classroom.Works, work)
	}
	for _, problem := range state.Problems {
		if !teacher && !allowedProblems[problem.ID] {
			continue
		}
		problem.Samples = slices.Clone(problem.Samples)
		if teacher {
			problem.Tests = slices.Clone(problem.Tests)
		} else {
			problem.Tests = nil
		}
		classroom.Problems = append(classroom.Problems, problem)
	}
	for _, submission := range state.Submissions {
		if teacher || (submission.UserID == user.ID && allowedWorks[submission.WorkID]) {
			submission.JudgedAt = copyTimestamp(submission.JudgedAt)
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

func copyTimestamp(timestamp *time.Time) *time.Time {
	if timestamp == nil {
		return nil
	}
	copy := *timestamp
	return &copy
}
