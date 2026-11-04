package domain_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestWorkVisibility(t *testing.T) {
	cases := []struct {
		name, role, group, status string
		visible                   bool
	}{
		{"teacher sees draft", "teacher", "other", "draft", true},
		{"teacher sees closed", "teacher", "", "closed", true},
		{"student cannot see draft", "student", "assigned", "draft", false},
		{"student sees open work", "student", "assigned", "open", true},
		{"student sees closed history", "student", "assigned", "closed", true},
		{"another group cannot see work", "student", "other", "open", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := model.Work{Status: tc.status, GroupIDs: []string{"assigned"}}
			user := model.User{Role: tc.role, GroupID: tc.group}
			if got := domain.WorkVisible(work, user); got != tc.visible {
				t.Fatalf("visibility = %v, want %v", got, tc.visible)
			}
		})
	}
}

func TestWorkStartedBoundary(t *testing.T) {
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	work := model.Work{StartsAt: &start}
	if domain.WorkStarted(work, start.Add(-time.Nanosecond)) {
		t.Fatal("work started before its start")
	}
	if !domain.WorkStarted(work, start) || !domain.WorkStarted(work, start.Add(time.Hour)) {
		t.Fatal("work did not start at its inclusive start boundary")
	}
	if !domain.WorkStarted(model.Work{}, start.Add(-time.Hour)) {
		t.Fatal("unscheduled work should already be available")
	}
	if !domain.WorkStarted(work, start.In(time.FixedZone("classroom", 3*60*60))) {
		t.Fatal("the same instant in a different zone changed start status")
	}
}

func TestSubmissionAdmission(t *testing.T) {
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	cases := []struct {
		name      string
		change    func(*model.Work, *model.User)
		problem   string
		now       time.Time
		wantError error
	}{
		{name: "at start", problem: "problem", now: start},
		{name: "before end", problem: "problem", now: end.Add(-time.Nanosecond)},
		{name: "before start", problem: "problem", now: start.Add(-time.Nanosecond), wantError: domain.ErrClosed},
		{name: "at end", problem: "problem", now: end, wantError: domain.ErrClosed},
		{name: "after end", problem: "problem", now: end.Add(time.Second), wantError: domain.ErrClosed},
		{name: "closed", change: func(w *model.Work, _ *model.User) { w.Status = "closed" }, problem: "problem", now: start, wantError: domain.ErrClosed},
		{name: "draft is hidden", change: func(w *model.Work, _ *model.User) { w.Status = "draft" }, problem: "problem", now: start, wantError: domain.ErrNotFound},
		{name: "other group is hidden", change: func(_ *model.Work, u *model.User) { u.GroupID = "other" }, problem: "problem", now: start, wantError: domain.ErrNotFound},
		{name: "missing work", change: func(w *model.Work, _ *model.User) { w.ID = "" }, problem: "problem", now: start, wantError: domain.ErrNotFound},
		{name: "teacher cannot submit", change: func(_ *model.Work, u *model.User) { u.Role = "teacher" }, problem: "problem", now: start, wantError: domain.ErrStudentOnly},
		{name: "unknown role cannot submit", change: func(_ *model.Work, u *model.User) { u.Role = "unknown" }, problem: "problem", now: start, wantError: domain.ErrStudentOnly},
		{name: "problem outside work", problem: "other", now: start, wantError: domain.ErrProblem},
		{name: "closed hides problem membership", problem: "other", now: end, wantError: domain.ErrClosed},
		{name: "unscheduled homework", change: func(w *model.Work, _ *model.User) { w.StartsAt, w.EndsAt = nil, nil }, problem: "problem", now: start},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := model.Work{ID: "work", Kind: "homework", Status: "open", GroupIDs: []string{"group"}, ProblemIDs: []string{"problem"}, StartsAt: &start, EndsAt: &end}
			user := model.User{ID: "student", Role: "student", GroupID: "group"}
			if tc.change != nil {
				tc.change(&work, &user)
			}
			if err := domain.CanSubmit(work, user, tc.problem, tc.now); !errors.Is(err, tc.wantError) {
				t.Fatalf("error = %v, want %v", err, tc.wantError)
			}
		})
	}
}

func TestValidateWork(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*model.State, *model.Work)
		invalid bool
	}{
		{name: "published homework"},
		{name: "optional topic", change: func(_ *model.State, w *model.Work) { w.TopicID = "" }},
		{name: "empty draft", change: func(_ *model.State, w *model.Work) { w.Status = "draft"; w.GroupIDs = nil; w.ProblemIDs = nil }},
		{name: "missing groups", change: func(_ *model.State, w *model.Work) { w.GroupIDs = nil }, invalid: true},
		{name: "missing problems", change: func(_ *model.State, w *model.Work) { w.ProblemIDs = nil }, invalid: true},
		{name: "closed work still needs assignments", change: func(_ *model.State, w *model.Work) { w.Status = "closed"; w.ProblemIDs = nil }, invalid: true},
		{name: "unknown topic", change: func(_ *model.State, w *model.Work) { w.TopicID = "missing" }, invalid: true},
		{name: "unknown group", change: func(_ *model.State, w *model.Work) { w.GroupIDs = []string{"missing"} }, invalid: true},
		{name: "unknown problem", change: func(_ *model.State, w *model.Work) { w.ProblemIDs = []string{"missing"} }, invalid: true},
		{name: "duplicate group", change: func(_ *model.State, w *model.Work) { w.GroupIDs = []string{"group", "group"} }, invalid: true},
		{name: "duplicate problem", change: func(_ *model.State, w *model.Work) { w.ProblemIDs = []string{"problem", "problem"} }, invalid: true},
		{name: "unknown kind", change: func(_ *model.State, w *model.Work) { w.Kind = "unknown" }, invalid: true},
		{name: "unknown status", change: func(_ *model.State, w *model.Work) { w.Status = "unknown" }, invalid: true},
		{name: "status default belongs to caller", change: func(_ *model.State, w *model.Work) { w.Status = "" }, invalid: true},
		{name: "short title", change: func(_ *model.State, w *model.Work) { w.Title = "a" }, invalid: true},
		{name: "whitespace title", change: func(_ *model.State, w *model.Work) { w.Title = "  " }, invalid: true},
		{name: "max title", change: func(_ *model.State, w *model.Work) { w.Title = strings.Repeat("a", 200) }},
		{name: "long title", change: func(_ *model.State, w *model.Work) { w.Title = strings.Repeat("a", 201) }, invalid: true},
		{name: "max description", change: func(_ *model.State, w *model.Work) { w.Description = strings.Repeat("a", 10000) }},
		{name: "long description", change: func(_ *model.State, w *model.Work) { w.Description = strings.Repeat("a", 10001) }, invalid: true},
		{name: "homework deadline without start", change: func(_ *model.State, w *model.Work) { end := time.Now(); w.EndsAt = &end }},
		{name: "contest missing times", change: func(_ *model.State, w *model.Work) { w.Kind = "contest" }, invalid: true},
		{name: "contest missing end", change: func(_ *model.State, w *model.Work) { start := time.Now(); w.Kind = "contest"; w.StartsAt = &start }, invalid: true},
		{name: "equal start and end", change: func(_ *model.State, w *model.Work) { start := time.Now(); w.StartsAt = &start; w.EndsAt = &start }, invalid: true},
		{name: "inverted schedule", change: func(_ *model.State, w *model.Work) {
			start := time.Now()
			end := start.Add(-time.Hour)
			w.StartsAt = &start
			w.EndsAt = &end
		}, invalid: true},
		{name: "scheduled contest", change: func(_ *model.State, w *model.Work) {
			start := time.Now()
			end := start.Add(time.Hour)
			w.Kind = "contest"
			w.StartsAt = &start
			w.EndsAt = &end
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, work := validWork()
			if tc.change != nil {
				tc.change(&state, &work)
			}
			if err := domain.ValidateWork(state, work); (err != nil) != tc.invalid {
				t.Fatalf("error = %v, invalid = %v", err, tc.invalid)
			}
		})
	}
}

func TestValidateWorkReferenceLimits(t *testing.T) {
	for _, count := range []int{50, 51} {
		t.Run(fmt.Sprintf("problems %d", count), func(t *testing.T) {
			state, work := validWork()
			state.Problems, work.ProblemIDs = nil, nil
			for i := range count {
				id := fmt.Sprint(i)
				state.Problems = append(state.Problems, model.Problem{ID: id})
				work.ProblemIDs = append(work.ProblemIDs, id)
			}
			if err := domain.ValidateWork(state, work); (err != nil) != (count > 50) {
				t.Fatalf("error = %v for %d problems", err, count)
			}
		})
	}
	for _, count := range []int{100, 101} {
		t.Run(fmt.Sprintf("groups %d", count), func(t *testing.T) {
			state, work := validWork()
			state.Groups, work.GroupIDs = nil, nil
			for i := range count {
				id := fmt.Sprint(i)
				state.Groups = append(state.Groups, model.Group{ID: id})
				work.GroupIDs = append(work.GroupIDs, id)
			}
			if err := domain.ValidateWork(state, work); (err != nil) != (count > 100) {
				t.Fatalf("error = %v for %d groups", err, count)
			}
		})
	}
}

func validWork() (model.State, model.Work) {
	state := model.State{Topics: []model.Topic{{ID: "topic"}}, Groups: []model.Group{{ID: "group"}}, Problems: []model.Problem{{ID: "problem"}}}
	work := model.Work{ID: "work", Title: "Homework", Kind: "homework", TopicID: "topic", GroupIDs: []string{"group"}, ProblemIDs: []string{"problem"}, Status: "open"}
	return state, work
}
