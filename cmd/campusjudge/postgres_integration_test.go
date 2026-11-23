package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/app"
	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
	"github.com/lioiioiioil/CampusJudge/internal/testsupport"
)

func legacyPostgresImportSource(t *testing.T) (string, model.State) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.json")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Date(2026, 11, 23, 12, 0, 0, 123456000, time.UTC)
	if _, err := app.Initialize(s, app.BootstrapOptions{TeacherPassword: "campus-integration-password", Now: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(state *model.State) error {
		state.Users = append(state.Users, model.User{ID: "learner", Name: "Ученик", Username: "learner", Role: "student", GroupID: state.Groups[0].ID, PasswordHash: "unused-test-hash"})
		state.Submissions = append(state.Submissions, model.Submission{
			ID: "running-submission", WorkID: state.Works[0].ID, ProblemID: state.Problems[0].ID,
			UserID: "learner", Language: "go", Source: "package main\n", Verdict: "pending", CreatedAt: now,
			Judging: &model.Judging{Status: "running", TotalTests: 3},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return path, s.Snapshot()
}

func TestPostgresCLIImportExitsBeforeListenerDockerAndBootstrap(t *testing.T) {
	dsn := testsupport.PostgresURL(t)
	path, want := legacyPostgresImportSource(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Neither the invalid listener nor a missing Docker executable is relevant
	// for the offline migration command. The queue must also remain running.
	err = serve(context.Background(), serverConfig{databaseURL: dsn, importJSON: path, addr: "missing-port", judging: "docker", judgeImage: "nonexistent-import-test-image"})
	if err != nil {
		t.Fatalf("offline import entered server startup: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := store.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	gotJSON, _ := json.Marshal(s.Snapshot())
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatal("CLI import changed classroom, credentials, or active queue")
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, before) {
		t.Fatal("CLI import modified the legacy source")
	}
}

func TestPostgresFailedListenerDoesNotModifyQueue(t *testing.T) {
	dsn := testsupport.PostgresURL(t)
	_, want := legacyPostgresImportSource(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := store.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(state *model.State) error { *state = want; return nil }); err != nil {
		_ = s.Close()
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	err = serve(context.Background(), serverConfig{databaseURL: dsn, addr: "missing-port", judging: "manual"})
	if err == nil || !strings.Contains(err.Error(), "listen HTTP") {
		t.Fatalf("startup error = %v, want listener failure", err)
	}
	s, err = store.OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	gotJSON, _ := json.Marshal(s.Snapshot())
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatal("failed PostgreSQL startup changed the persisted queue")
	}
}
