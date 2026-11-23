package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
	"github.com/lioiioiioil/CampusJudge/internal/testsupport"
)

func postgresTestDatabase(t *testing.T) string {
	t.Helper()
	return testsupport.PostgresURL(t)
}

func openPostgresStore(t *testing.T, dsn string) *store.Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := store.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("close PostgreSQL store: %v", err)
		}
	})
	return s
}

func postgresState() model.State {
	now := time.Date(2026, 11, 23, 12, 34, 56, 123456000, time.UTC)
	start, end, judged := now.Add(-time.Hour), now.Add(time.Hour), now.Add(-time.Minute)
	return model.State{
		Demo: true, Version: 1,
		Groups: []model.Group{{ID: "group-z", Name: "Первая группа", InviteCode: "CLASS-Z"}, {ID: "group-a", Name: "Вторая группа", InviteCode: "CLASS-A"}},
		Users: []model.User{
			{ID: "teacher-z", Name: "Преподаватель", Username: "teacher", Role: "teacher", PasswordHash: "teacher-hash"},
			{ID: "student-a", Name: "Ученик", Username: "learner", Role: "student", GroupID: "group-z", PasswordHash: "student-hash"},
		},
		Topics: []model.Topic{{ID: "topic", Title: "Циклы", Description: "Описание", LessonURL: "https://example.com/lesson"}},
		Problems: []model.Problem{{
			ID: "problem", Title: "Сумма", Statement: "Сложите числа", InputFormat: "Два числа", OutputFormat: "Одно число", TimeLimitMS: 1500, MemoryLimitMB: 128,
			Samples: []model.TestCase{{Input: "1 2\n", Output: "3\n"}}, Tests: []model.TestCase{{Input: "-2 2\n", Output: "0\n"}},
			TestRevisions: []model.TestRevision{
				{Number: 1, Samples: []model.TestCase{{Input: "1 2\n", Output: "3\n"}}, Tests: []model.TestCase{{Input: "-2 2\n", Output: "0\n"}}, CreatedAt: now.Add(-time.Hour)},
				{Number: 2, Samples: []model.TestCase{{Input: "2 3\n", Output: "5\n"}}, Tests: []model.TestCase{{Input: "0 0\n", Output: "0\n"}}, CreatedAt: now},
			},
		}},
		Works: []model.Work{{ID: "work", Title: "Контест", Description: "Описание работы", Kind: "contest", TopicID: "topic", GroupIDs: []string{"group-z", "group-a"}, ProblemIDs: []string{"problem"}, ProblemRevisions: map[string]int{"problem": 2}, Status: "open", StartsAt: &start, EndsAt: &end, CreatedAt: now}},
		Submissions: []model.Submission{{
			ID: "submission", WorkID: "work", ProblemID: "problem", UserID: "student-a", Language: "go", Source: "package main\n// учебный код\n", Verdict: "wrong_answer", Feedback: "Попробуйте ещё раз", CreatedAt: now, JudgedAt: &judged, TestRevision: 1,
			Judging:       &model.Judging{Status: "running", TargetRevision: 2, PassedTests: 1, TotalTests: 2, FailedTest: 2, TimeMS: 7, CompileOutput: "diagnostic", Error: "previous error"},
			PastJudgments: []model.PastJudgment{{Verdict: "accepted", TestRevision: 1, ReplacedAt: judged}},
		}},
		Sessions: []model.Session{{TokenHash: "session-token-hash", UserID: "student-a", ExpiresAt: end}},
	}
}

func assertPostgresState(t *testing.T, got, want model.State) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("PostgreSQL state did not roundtrip\ngot:  %s\nwant: %s", gotJSON, wantJSON)
	}
}

func TestPostgresStateRoundtripAndExclusiveOwnership(t *testing.T) {
	dsn := postgresTestDatabase(t)
	s := openPostgresStore(t, dsn)
	want := postgresState()
	local := time.Date(2026, 11, 23, 15, 34, 56, 123456789, time.FixedZone("UTC+3", 3*60*60))
	want.Works[0].CreatedAt = local
	want.Submissions[0].PastJudgments[0].ReplacedAt = local
	if err := s.Update(func(state *model.State) error { *state = want; return nil }); err != nil {
		t.Fatal(err)
	}
	canonical := time.Date(2026, 11, 23, 12, 34, 56, 123456000, time.UTC)
	want.Works[0].CreatedAt = canonical
	want.Submissions[0].PastJudgments[0].ReplacedAt = canonical
	assertPostgresState(t, s.Snapshot(), want)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	contender, err := store.OpenPostgres(ctx, dsn)
	if contender != nil {
		_ = contender.Close()
	}
	if err == nil {
		t.Fatal("second server acquired the same PostgreSQL classroom")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(*model.State) error { return nil }); err == nil {
		t.Fatal("closed PostgreSQL store accepted an update")
	}
	reopened := openPostgresStore(t, dsn)
	assertPostgresState(t, reopened.Snapshot(), want)
}

func TestPostgresRollbackDoesNotPublishPartialState(t *testing.T) {
	dsn := postgresTestDatabase(t)
	s := openPostgresStore(t, dsn)
	want := postgresState()
	if err := s.Update(func(state *model.State) error { *state = want; return nil }); err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("teacher cancelled edit")
	err := s.Update(func(state *model.State) error {
		state.Groups[0].Name = "Rejected edit"
		return rejected
	})
	if !errors.Is(err, rejected) {
		t.Fatalf("callback rejection was lost: %v", err)
	}
	assertPostgresState(t, s.Snapshot(), want)
	// The new group precedes the duplicate account in the persistence order.
	// A database uniqueness violation must roll back both changes together.
	err = s.Update(func(state *model.State) error {
		state.Groups = append(state.Groups, model.Group{ID: "rolled-back-group", Name: "Must not survive", InviteCode: "ROLLBACK"})
		state.Users = append(state.Users, model.User{ID: "duplicate-account", Name: "Duplicate", Username: "teacher", Role: "teacher", PasswordHash: "unused-hash"})
		return nil
	})
	if err == nil {
		t.Fatal("duplicate username did not fail the database transaction")
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("constraint failure left an unusable connection: %v", err)
	}
	select {
	case err := <-s.Failure():
		t.Fatalf("ordinary transaction rejection reported fatal storage loss: %v", err)
	default:
	}
	assertPostgresState(t, s.Snapshot(), want)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	assertPostgresState(t, openPostgresStore(t, dsn).Snapshot(), want)
}

func TestPostgresConcurrentUpdatesRemainDurable(t *testing.T) {
	dsn := postgresTestDatabase(t)
	s := openPostgresStore(t, dsn)
	const writers = 12
	var workers sync.WaitGroup
	errors := make(chan error, writers)
	for i := range writers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			errors <- s.Update(func(state *model.State) error {
				state.Groups = append(state.Groups, model.Group{ID: fmt.Sprintf("group-%02d", i), Name: fmt.Sprintf("Группа %d", i), InviteCode: fmt.Sprintf("CLASS-%02d", i)})
				return nil
			})
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := s.Snapshot()
	if len(want.Groups) != writers {
		t.Fatalf("lost updates: got %d groups, want %d", len(want.Groups), writers)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	assertPostgresState(t, openPostgresStore(t, dsn).Snapshot(), want)
}

func TestPostgresImportsLegacyJSONWithoutModifyingSource(t *testing.T) {
	dsn := postgresTestDatabase(t)
	s := openPostgresStore(t, dsn)
	path := filepath.Join(t.TempDir(), "classroom.json")
	if err := s.ImportJSON(path); err == nil {
		t.Fatal("missing import source was accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("import created a missing source file")
	}
	for _, invalid := range []string{"invalid JSON", `{"version":999}`, `{"version":1} {"version":1}`} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := s.ImportJSON(path); err == nil {
			t.Fatal("invalid import source was accepted")
		}
		if len(s.Snapshot().Users) != 0 || len(s.Snapshot().Groups) != 0 {
			t.Fatal("failed import published data")
		}
	}
	want := postgresState()
	source, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	jsonOwner, err := store.OpenExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ImportJSON(path); err == nil {
		_ = jsonOwner.Close()
		t.Fatal("import accepted a JSON source owned by a running server")
	}
	if err := jsonOwner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.ImportJSON(path); err != nil {
		t.Fatal(err)
	}
	assertPostgresState(t, s.Snapshot(), want)
	if err := s.ImportJSON(path); err == nil {
		t.Fatal("import overwrote a nonempty PostgreSQL classroom")
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, source) {
		t.Fatal("import modified the JSON source")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	assertPostgresState(t, openPostgresStore(t, dsn).Snapshot(), want)
}

func TestPostgresRejectsUnknownSchemaWithoutChangingItOrRetainingLock(t *testing.T) {
	dsn := postgresTestDatabase(t)
	s := openPostgresStore(t, dsn)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	connection, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("connect to test database failed")
	}
	defer connection.Close(context.Background())
	if _, err := connection.Exec(ctx, "UPDATE campusjudge.metadata SET schema_version=999"); err != nil {
		t.Fatal(err)
	}
	unsupported, err := store.OpenPostgres(ctx, dsn)
	if unsupported != nil {
		_ = unsupported.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("unknown schema was accepted or mislabeled: %v", err)
	}
	var version int
	if err := connection.QueryRow(ctx, "SELECT schema_version FROM campusjudge.metadata").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 999 {
		t.Fatal("opening an unsupported schema changed its version")
	}
	if _, err := connection.Exec(ctx, "UPDATE campusjudge.metadata SET schema_version=1"); err != nil {
		t.Fatal(err)
	}
	_ = openPostgresStore(t, dsn)
}

func TestPostgresLostConnectionCannotOverwriteNewOwner(t *testing.T) {
	dsn := postgresTestDatabase(t)
	stale := openPostgresStore(t, dsn)
	want := postgresState()
	if err := stale.Update(func(state *model.State) error { *state = want; return nil }); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("connect to disposable PostgreSQL server failed")
	}
	defer admin.Close(context.Background())
	var terminated bool
	if err := admin.QueryRow(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND application_name='CampusJudge'").Scan(&terminated); err != nil || !terminated {
		t.Fatalf("terminate old owner: terminated=%v, error=%v", terminated, err)
	}
	staleWrite := func(state *model.State) error { state.Groups[0].Name = "Stale owner edit"; return nil }
	if err := stale.Update(staleWrite); err == nil {
		t.Fatal("disconnected owner accepted a write")
	}
	select {
	case err := <-stale.Failure():
		if err == nil {
			t.Fatal("lost owner emitted an empty failure")
		}
	case <-time.After(time.Second):
		t.Fatal("lost owner did not notify the service to restart")
	}
	assertPostgresState(t, stale.Snapshot(), want)
	fresh := openPostgresStore(t, dsn)
	if err := fresh.Update(func(state *model.State) error { state.Groups[0].Name = "New owner edit"; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := stale.Update(staleWrite); err == nil {
		t.Fatal("old owner reconnected and accepted a stale write")
	}
	assertPostgresState(t, stale.Snapshot(), want)
	want.Groups[0].Name = "New owner edit"
	if err := fresh.Close(); err != nil {
		t.Fatal(err)
	}
	assertPostgresState(t, openPostgresStore(t, dsn).Snapshot(), want)
}

func TestPostgresReordersWorkGroupsWithoutUniquenessConflict(t *testing.T) {
	dsn := postgresTestDatabase(t)
	s := openPostgresStore(t, dsn)
	want := postgresState()
	if err := s.Update(func(state *model.State) error { *state = want; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(state *model.State) error {
		state.Works[0].GroupIDs[0], state.Works[0].GroupIDs[1] = state.Works[0].GroupIDs[1], state.Works[0].GroupIDs[0]
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want.Works[0].GroupIDs = []string{"group-a", "group-z"}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	assertPostgresState(t, openPostgresStore(t, dsn).Snapshot(), want)
}
