package domain_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestStandingsUseChronologyAndICPCPenalties(t *testing.T) {
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	work := model.Work{ID: "contest", Kind: "contest", GroupIDs: []string{"group"}, ProblemIDs: []string{"a", "b"}, StartsAt: &start}
	users := []model.User{
		{ID: "lena", Name: "Лена", Role: "student", GroupID: "group"},
		{ID: "anna", Name: "Анна", Role: "student", GroupID: "group"},
		{ID: "boris", Name: "Борис", Role: "student", GroupID: "group"},
		{ID: "teacher", Name: "Преподаватель", Role: "teacher", GroupID: "group"},
		{ID: "outsider", Name: "Другая группа", Role: "student", GroupID: "other"},
	}
	makeSubmission := func(id, user, problem, verdict string, offset time.Duration) model.Submission {
		return model.Submission{ID: id, WorkID: work.ID, UserID: user, ProblemID: problem, Verdict: verdict, CreatedAt: start.Add(offset)}
	}
	// Storage order deliberately differs from submission time, as can happen
	// when two requests begin together and acquire the write lock out of order.
	submissions := []model.Submission{
		makeSubmission("lena-ac-b", "lena", "b", "accepted", 40*time.Minute),
		makeSubmission("lena-wa", "lena", "a", "wrong_answer", 2*time.Minute),
		makeSubmission("anna-ac-b", "anna", "b", "accepted", 41*time.Minute),
		makeSubmission("lena-ac-a", "lena", "a", "accepted", 17*time.Minute+59*time.Second),
		makeSubmission("lena-late-wa", "lena", "a", "wrong_answer", 20*time.Minute),
		makeSubmission("lena-ce", "lena", "a", "compilation_error", 3*time.Minute),
		makeSubmission("lena-pending", "lena", "a", "pending", 4*time.Minute),
		makeSubmission("lena-re", "lena", "a", "runtime_error", 6*time.Minute),
		makeSubmission("anna-ac-a", "anna", "a", "accepted", 12*time.Minute),
		makeSubmission("anna-wa-b", "anna", "b", "wrong_answer", 11*time.Minute),
		makeSubmission("boris-wa", "boris", "a", "wrong_answer", time.Minute),
		makeSubmission("boris-ce", "boris", "a", "compilation_error", 2*time.Minute),
		makeSubmission("boris-pending", "boris", "a", "pending", 3*time.Minute),
		makeSubmission("outsider-ac", "outsider", "a", "accepted", time.Minute),
		makeSubmission("teacher-ac", "teacher", "a", "accepted", time.Minute),
		{ID: "other-work", WorkID: "other", UserID: "boris", ProblemID: "a", Verdict: "accepted", CreatedAt: start.Add(time.Minute)},
	}
	before := slices.Clone(submissions)
	got := domain.ComputeStandings(work, users, submissions)
	// Anna: 12 + (41 + 20) = 73. Lena: (17 + 2*20) + 40 = 97.
	// Boris's unsolved problem contributes no penalty.
	want := []domain.StandingRow{
		{UserID: "anna", Name: "Анна", Solved: 2, Penalty: 73, Problems: map[string]*domain.ProblemScore{"a": {Solved: true, Minutes: 12}, "b": {Solved: true, Attempts: 1, Minutes: 41}}},
		{UserID: "lena", Name: "Лена", Solved: 2, Penalty: 97, Problems: map[string]*domain.ProblemScore{"a": {Solved: true, Attempts: 2, Minutes: 17}, "b": {Solved: true, Minutes: 40}}},
		{UserID: "boris", Name: "Борис", Problems: map[string]*domain.ProblemScore{"a": {Attempts: 1}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("standings = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(submissions, before) {
		t.Fatal("calculation reordered the caller's submissions")
	}
}

func TestEqualTimeSubmissionsPreserveArrivalOrder(t *testing.T) {
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	work := model.Work{ID: "work", Kind: "contest", StartsAt: &start, GroupIDs: []string{"group"}}
	users := []model.User{{ID: "student", Role: "student", GroupID: "group"}}
	failed := model.Submission{ID: "z", WorkID: "work", UserID: "student", ProblemID: "a", Verdict: "wrong_answer", CreatedAt: start.Add(10 * time.Minute)}
	accepted := failed
	accepted.ID, accepted.Verdict = "a", "accepted"
	first := domain.ComputeStandings(work, users, []model.Submission{failed, accepted})
	if first[0].Penalty != 30 || first[0].Problems["a"].Attempts != 1 {
		t.Fatal("failure before acceptance at the same instant must add 20 minutes")
	}
	second := domain.ComputeStandings(work, users, []model.Submission{accepted, failed})
	if second[0].Penalty != 10 || second[0].Problems["a"].Attempts != 0 {
		t.Fatal("failure recorded after acceptance must not affect the score")
	}
}

func TestHomeworkHasNoTimePenalty(t *testing.T) {
	start := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	work := model.Work{ID: "work", Kind: "homework", StartsAt: &start, GroupIDs: []string{"group"}}
	users := []model.User{{ID: "student", Role: "student", GroupID: "group"}}
	submissions := []model.Submission{
		{ID: "one", WorkID: "work", UserID: "student", ProblemID: "a", Verdict: "time_limit", CreatedAt: start.Add(time.Minute)},
		{ID: "two", WorkID: "work", UserID: "student", ProblemID: "a", Verdict: "accepted", CreatedAt: start.Add(time.Hour)},
	}
	got := domain.ComputeStandings(work, users, submissions)
	if got[0].Solved != 1 || got[0].Penalty != 0 || got[0].Problems["a"].Minutes != 0 || got[0].Problems["a"].Attempts != 1 {
		t.Fatalf("homework result = %+v (%+v)", got[0], got[0].Problems["a"])
	}
}

func TestStandingsTiesHaveDeterministicUserOrder(t *testing.T) {
	work := model.Work{ID: "work", GroupIDs: []string{"group"}}
	users := []model.User{{ID: "z", Name: "Same", Role: "student", GroupID: "group"}, {ID: "a", Name: "Same", Role: "student", GroupID: "group"}}
	got := domain.ComputeStandings(work, users, nil)
	if len(got) != 2 || got[0].UserID != "a" || got[1].UserID != "z" {
		t.Fatalf("tied rows = %+v", got)
	}
	if empty := domain.ComputeStandings(work, nil, nil); empty == nil || len(empty) != 0 {
		t.Fatalf("empty roster should serialize as an empty list: %#v", empty)
	}
}
