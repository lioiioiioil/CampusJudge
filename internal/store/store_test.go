package store_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

func openStore(t *testing.T, path string) *store.Store {
	t.Helper()
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUpdatePersistsAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "state.json")
	s := openStore(t, path)
	initial := s.Snapshot()
	if initial.Version != 1 || initial.Users == nil || initial.Works == nil {
		t.Fatalf("unexpected initial state: %+v", initial)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open created or could not inspect file: %v", err)
	}
	if err := s.Update(func(state *model.State) error {
		state.Users = append(state.Users, model.User{ID: "student", Name: "Ученик"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got := openStore(t, path).Snapshot()
	if len(got.Users) != 1 || got.Users[0].ID != "student" || got.Users[0].Name != "Ученик" {
		t.Fatalf("data was not preserved: %+v", got.Users)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("state file permissions = %o, want 600", fileInfo.Mode().Perm())
	}
	directoryInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("state directory permissions = %o, want 700", directoryInfo.Mode().Perm())
	}
}

func TestFailedTransactionDoesNotChangeMemoryOrDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := openStore(t, path)
	if err := s.Update(func(state *model.State) error {
		state.Groups = []model.Group{{ID: "original"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("rejected")
	err = s.Update(func(state *model.State) error {
		state.Groups[0].ID = "modified"
		state.Users = append(state.Users, model.User{ID: "unexpected"})
		return rejected
	})
	if !errors.Is(err, rejected) {
		t.Fatalf("error = %v, want original callback error", err)
	}
	if got := s.Snapshot(); got.Groups[0].ID != "original" || len(got.Users) != 0 {
		t.Fatalf("rejected transaction changed memory: %+v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rejected transaction changed state file")
	}
}

func TestSnapshotsAndTransactionInputsAreIndependent(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "state.json"))
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	var retained *model.State
	if err := s.Update(func(state *model.State) error {
		state.Problems = []model.Problem{{ID: "problem", Samples: []model.TestCase{{Input: "sample"}}, Tests: []model.TestCase{{Input: "hidden"}}}}
		state.Works = []model.Work{{ID: "work", ProblemIDs: []string{"problem"}, GroupIDs: []string{"group"}, StartsAt: &now, EndsAt: &now}}
		state.Submissions = []model.Submission{{ID: "submission", JudgedAt: &now, Judging: &model.Judging{Status: "done", PassedTests: 3}}}
		retained = state
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	retained.Problems[0].Tests[0].Input = "mutated through callback"
	retained.Works[0].GroupIDs[0] = "other group"
	*retained.Works[0].StartsAt = now.Add(time.Hour)
	retained.Submissions[0].Judging.Status = "changed"
	copy := s.Snapshot()
	copy.Problems[0].Samples[0].Input = "mutated through snapshot"
	copy.Works[0].ProblemIDs[0] = "other problem"
	*copy.Works[0].EndsAt = now.Add(time.Hour)
	*copy.Submissions[0].JudgedAt = now.Add(time.Hour)
	copy.Submissions[0].Judging.PassedTests = 100
	got := s.Snapshot()
	if got.Problems[0].Tests[0].Input != "hidden" || got.Problems[0].Samples[0].Input != "sample" ||
		got.Works[0].GroupIDs[0] != "group" || got.Works[0].ProblemIDs[0] != "problem" ||
		!got.Works[0].StartsAt.Equal(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)) ||
		!got.Works[0].EndsAt.Equal(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)) ||
		!got.Submissions[0].JudgedAt.Equal(time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)) ||
		got.Submissions[0].Judging.Status != "done" || got.Submissions[0].Judging.PassedTests != 3 {
		t.Fatalf("caller mutated stored data: %+v", got)
	}
}

func TestConcurrentUpdatesDoNotLoseData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := openStore(t, path)
	const writers, perWriter = 12, 10
	var tasks sync.WaitGroup
	errorsFound := make(chan error, writers)
	for writer := range writers {
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			for item := range perWriter {
				id := fmt.Sprintf("%d-%d", writer, item)
				if err := s.Update(func(state *model.State) error {
					state.Topics = append(state.Topics, model.Topic{ID: id})
					return nil
				}); err != nil {
					errorsFound <- err
					return
				}
				_ = s.Snapshot()
			}
		}()
	}
	tasks.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	got := openStore(t, path).Snapshot()
	seen := make(map[string]bool)
	for _, topic := range got.Topics {
		seen[topic.ID] = true
	}
	if len(got.Topics) != writers*perWriter || len(seen) != writers*perWriter {
		t.Fatalf("got %d topics (%d distinct), want %d", len(got.Topics), len(seen), writers*perWriter)
	}
}

func TestRejectsCorruptAndUnsupportedFiles(t *testing.T) {
	cases := []string{"", "{", "null", `{}`, `{"version":2}`, `{"version":1} {"version":1}`, `{"version":1} trailing`, `{"version":1,"unrecognized":true}`}
	for _, content := range cases {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Open(path); err == nil {
				t.Fatal("invalid file was accepted")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Fatal("Open overwrote invalid data")
			}
		})
	}
}

func TestEncodingFailurePreservesStateAndCleansTemporaryFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "state.json")
	s := openStore(t, path)
	if err := s.Update(func(state *model.State) error {
		state.Topics = []model.Topic{{ID: "original"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Update(func(state *model.State) error {
		state.Topics[0].ID = "modified"
		state.Works = []model.Work{{CreatedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}
		return nil
	})
	if err == nil {
		t.Fatal("invalid JSON timestamp unexpectedly persisted")
	}
	if got := s.Snapshot(); got.Topics[0].ID != "original" || len(got.Works) != 0 {
		t.Fatal("failed save changed memory")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed save changed previous state file")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("temporary file leaked: %v", entries)
	}
}

func TestFileSystemFailurePreservesPublishedState(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "data")
	path := filepath.Join(directory, "state.json")
	s := openStore(t, path)
	if err := s.Update(func(state *model.State) error {
		state.Groups = []model.Group{{ID: "original"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate a storage path becoming unavailable without relying on chmod,
	// which is ineffective for processes running with elevated permissions.
	backupDirectory := filepath.Join(parent, "saved-data")
	if err := os.Rename(directory, backupDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory, []byte("occupied path"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := s.Update(func(state *model.State) error {
		state.Groups[0].ID = "modified"
		return nil
	})
	if err == nil {
		t.Fatal("save to unavailable directory unexpectedly succeeded")
	}
	if s.Snapshot().Groups[0].ID != "original" {
		t.Fatal("failed write changed published memory")
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupDirectory, directory); err != nil {
		t.Fatal(err)
	}
	if openStore(t, path).Snapshot().Groups[0].ID != "original" {
		t.Fatal("failed write changed previous on-disk data")
	}
}
