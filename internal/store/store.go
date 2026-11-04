// Package store provides transactional JSON-file storage for one server process.
// Multiple processes must not open the same state file for writing.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

const currentVersion = 1

type Store struct {
	mu    sync.RWMutex
	path  string
	state model.State
}

// Open reads an existing state file or prepares an empty state if it is missing.
// A missing file is created by the first successful Update, never by a read.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("state file path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve state file: %w", err)
	}
	s := &Store{path: absPath, state: model.State{Version: currentVersion}}
	normalize(&s.state)
	file, err := os.Open(absPath)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open state file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var loaded model.State
	if err := decoder.Decode(&loaded); err != nil {
		return nil, fmt.Errorf("decode state file: %w", err)
	}
	// Reject concatenated documents and trailing garbage instead of silently
	// discarding data that a subsequent Update would otherwise overwrite.
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("state file must contain exactly one JSON document")
	}
	if loaded.Version != currentVersion {
		return nil, fmt.Errorf("unsupported state version %d (expected %d)", loaded.Version, currentVersion)
	}
	s.state = loaded
	normalize(&s.state)
	return s, nil
}

// Snapshot returns an independent copy, including nested slices and timestamps.
func (s *Store) Snapshot() model.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.state)
}

// Update serializes transactions and commits only after the file was replaced.
// The callback receives a private copy and must not call this Store recursively.
// A callback or storage error leaves the published in-memory state unchanged.
func (s *Store) Update(fn func(*model.State) error) error {
	if fn == nil {
		return errors.New("transaction callback is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate := clone(s.state)
	if err := fn(&candidate); err != nil {
		return err
	}
	if candidate.Version != currentVersion {
		return fmt.Errorf("unsupported state version %d (expected %d)", candidate.Version, currentVersion)
	}
	// The callback may retain its input. Keep that input separate from the
	// committed state so later caller mutations cannot bypass persistence.
	committed := clone(candidate)
	if err := s.persist(committed); err != nil {
		return err
	}
	s.state = committed
	return nil
}

func (s *Store) persist(state model.State) error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	file, err := os.CreateTemp(directory, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync state file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close state file: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	return nil
}

func clone(state model.State) model.State {
	copy := state
	copy.Users = slices.Clone(state.Users)
	copy.Groups = slices.Clone(state.Groups)
	copy.Topics = slices.Clone(state.Topics)
	copy.Problems = slices.Clone(state.Problems)
	for i := range copy.Problems {
		copy.Problems[i].Samples = slices.Clone(state.Problems[i].Samples)
		copy.Problems[i].Tests = slices.Clone(state.Problems[i].Tests)
	}
	copy.Works = slices.Clone(state.Works)
	for i := range copy.Works {
		copy.Works[i].GroupIDs = slices.Clone(state.Works[i].GroupIDs)
		copy.Works[i].ProblemIDs = slices.Clone(state.Works[i].ProblemIDs)
		if state.Works[i].StartsAt != nil {
			timestamp := *state.Works[i].StartsAt
			copy.Works[i].StartsAt = &timestamp
		}
		if state.Works[i].EndsAt != nil {
			timestamp := *state.Works[i].EndsAt
			copy.Works[i].EndsAt = &timestamp
		}
	}
	copy.Submissions = slices.Clone(state.Submissions)
	for i := range copy.Submissions {
		if state.Submissions[i].JudgedAt != nil {
			timestamp := *state.Submissions[i].JudgedAt
			copy.Submissions[i].JudgedAt = &timestamp
		}
	}
	copy.Sessions = slices.Clone(state.Sessions)
	normalize(&copy)
	return copy
}

func normalize(state *model.State) {
	if state.Users == nil {
		state.Users = []model.User{}
	}
	if state.Groups == nil {
		state.Groups = []model.Group{}
	}
	if state.Topics == nil {
		state.Topics = []model.Topic{}
	}
	if state.Problems == nil {
		state.Problems = []model.Problem{}
	}
	if state.Works == nil {
		state.Works = []model.Work{}
	}
	if state.Submissions == nil {
		state.Submissions = []model.Submission{}
	}
	if state.Sessions == nil {
		state.Sessions = []model.Session{}
	}
}
