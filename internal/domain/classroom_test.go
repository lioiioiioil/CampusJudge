package domain_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/domain"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestStudentClassroomDoesNotDisclosePrivateData(t *testing.T) {
	state, now := classroomFixture()
	got := domain.ClassroomFor(state, state.Users[1], now)
	if got.User.ID != "alice" || len(got.Users) != 1 || got.Users[0].ID != "alice" {
		t.Fatalf("student profiles = %+v", got.Users)
	}
	if len(got.Groups) != 1 || got.Groups[0].ID != "group" || got.Groups[0].InviteCode != "" {
		t.Fatalf("student groups = %+v", got.Groups)
	}
	workIDs := make([]string, 0)
	for _, work := range got.Works {
		workIDs = append(workIDs, work.ID)
	}
	if !reflect.DeepEqual(workIDs, []string{"open", "future", "closed"}) {
		t.Fatalf("visible work cards = %v", workIDs)
	}
	problemIDs := make([]string, 0)
	for _, problem := range got.Problems {
		problemIDs = append(problemIDs, problem.ID)
		if len(problem.Tests) != 0 {
			t.Fatal("student received hidden tests")
		}
	}
	if !reflect.DeepEqual(problemIDs, []string{"visible", "closed"}) {
		t.Fatalf("visible problems = %v", problemIDs)
	}
	if len(got.Submissions) != 2 || got.Submissions[0].ID != "alice-open" || got.Submissions[1].ID != "alice-closed" {
		t.Fatalf("visible submissions = %+v", got.Submissions)
	}
	if len(got.Topics) != 2 || got.Topics[1].ID != "unassigned-topic" {
		t.Fatal("shared topics were coupled to homework publication")
	}
	serialized, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"password_hash", "PRIVATE_HASH", "PRIVATE_INVITE", "PRIVATE_HIDDEN_TEST", "BOB_PRIVATE_CODE", "PRIVATE_DRAFT_CODE", "PRIVATE_OTHER_GROUP_CODE", "PRIVATE_FUTURE_STATEMENT", "PRIVATE_DRAFT_STATEMENT", "PRIVATE_OTHER_GROUP_STATEMENT", "PRIVATE_ORPHAN_STATEMENT"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("student response contains %q", secret)
		}
	}
}

func TestTeacherClassroomIncludesTeachingDataWithoutCredentials(t *testing.T) {
	state, now := classroomFixture()
	got := domain.ClassroomFor(state, state.Users[0], now)
	if len(got.Users) != len(state.Users) || len(got.Groups) != len(state.Groups) || len(got.Topics) != len(state.Topics) ||
		len(got.Works) != len(state.Works) || len(got.Problems) != len(state.Problems) || len(got.Submissions) != len(state.Submissions) {
		t.Fatal("teacher did not receive complete teaching data")
	}
	if got.Groups[0].InviteCode != "PRIVATE_INVITE" || got.Problems[0].Tests[0].Output != "PRIVATE_HIDDEN_TEST" {
		t.Fatal("teacher lost invitation codes or hidden tests")
	}
	serialized, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(serialized, []byte("password_hash")) || bytes.Contains(serialized, []byte("PRIVATE_HASH")) {
		t.Fatal("teacher projection includes password hashes")
	}
}

func TestClassroomProjectionDoesNotMutateOrAliasInput(t *testing.T) {
	state, now := classroomFixture()
	before, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	student := domain.ClassroomFor(state, state.Users[1], now)
	student.Problems[0].Samples[0].Input = "changed"
	student.Works[0].GroupIDs[0] = "changed"
	student.Works[0].ProblemIDs[0] = "changed"
	*student.Works[1].StartsAt = now.Add(2 * time.Hour)
	*student.Submissions[0].JudgedAt = now.Add(time.Hour)
	teacher := domain.ClassroomFor(state, state.Users[0], now)
	teacher.Problems[0].Tests[0].Output = "changed"
	teacher.Groups[0].InviteCode = "changed"
	after, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("projection or mutation of its result changed input state")
	}
}

func TestEmptyClassroomListsAreNotNull(t *testing.T) {
	got := domain.ClassroomFor(model.State{}, model.User{}, time.Time{})
	if got.Users == nil || got.Groups == nil || got.Topics == nil || got.Problems == nil || got.Works == nil || got.Submissions == nil {
		t.Fatalf("empty projection contains null lists: %#v", got)
	}
}

func classroomFixture() (model.State, time.Time) {
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	state := model.State{
		Users: []model.User{
			{ID: "teacher", Role: "teacher", PasswordHash: "PRIVATE_HASH_TEACHER"},
			{ID: "alice", Role: "student", GroupID: "group", PasswordHash: "PRIVATE_HASH_ALICE"},
			{ID: "bob", Role: "student", GroupID: "group", PasswordHash: "PRIVATE_HASH_BOB"},
		},
		Groups: []model.Group{{ID: "group", InviteCode: "PRIVATE_INVITE"}, {ID: "other-group", InviteCode: "PRIVATE_INVITE_OTHER"}},
		Topics: []model.Topic{{ID: "topic"}, {ID: "unassigned-topic"}},
		Problems: []model.Problem{
			{ID: "visible", Statement: "Open problem", Samples: []model.TestCase{{Input: "sample"}}, Tests: []model.TestCase{{Output: "PRIVATE_HIDDEN_TEST"}}},
			{ID: "draft", Statement: "PRIVATE_DRAFT_STATEMENT"},
			{ID: "future", Statement: "PRIVATE_FUTURE_STATEMENT"},
			{ID: "closed", Statement: "Closed problem"},
			{ID: "other-group", Statement: "PRIVATE_OTHER_GROUP_STATEMENT"},
			{ID: "orphan", Statement: "PRIVATE_ORPHAN_STATEMENT"},
		},
		Works: []model.Work{
			{ID: "open", Status: "open", GroupIDs: []string{"group"}, ProblemIDs: []string{"visible"}},
			{ID: "draft", Status: "draft", GroupIDs: []string{"group"}, ProblemIDs: []string{"draft"}},
			{ID: "future", Status: "open", StartsAt: &future, GroupIDs: []string{"group"}, ProblemIDs: []string{"future"}},
			{ID: "closed", Status: "closed", EndsAt: &past, GroupIDs: []string{"group"}, ProblemIDs: []string{"closed"}},
			{ID: "other-group", Status: "open", GroupIDs: []string{"other-group"}, ProblemIDs: []string{"other-group"}},
		},
		Submissions: []model.Submission{
			{ID: "alice-open", UserID: "alice", WorkID: "open", Source: "own source", JudgedAt: &now},
			{ID: "bob-open", UserID: "bob", WorkID: "open", Source: "BOB_PRIVATE_CODE"},
			{ID: "alice-draft", UserID: "alice", WorkID: "draft", Source: "PRIVATE_DRAFT_CODE"},
			{ID: "alice-other", UserID: "alice", WorkID: "other-group", Source: "PRIVATE_OTHER_GROUP_CODE"},
			{ID: "alice-closed", UserID: "alice", WorkID: "closed", Source: "old own source"},
		},
	}
	return state, now
}
