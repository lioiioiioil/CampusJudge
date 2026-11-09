package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrStateInUse = errors.New("state file is already in use by another CampusJudge instance")

// OpenExclusive holds a process-level lock until Close. The lock lives on a
// separate inode because committing a transaction replaces the state file.
func OpenExclusive(path string) (*Store, error) {
	path, err := ownedStatePath(path)
	if err != nil {
		return nil, err
	}
	owner, err := lockStateFile(path + ".lock")
	if err != nil {
		return nil, err
	}
	s, err := openState(path)
	if err != nil {
		return nil, errors.Join(err, owner.Close())
	}
	s.owner = owner
	return s, nil
}

func ownedStatePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("state file path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve state file: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve state file: %w", err)
	}
	if _, err := os.Lstat(absolute); err == nil {
		return "", errors.New("state file symlink target does not exist")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect state path: %w", err)
	}
	directory := filepath.Dir(absolute)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", fmt.Errorf("create state directory: %w", err)
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return "", fmt.Errorf("resolve state directory: %w", err)
	}
	return filepath.Join(directory, filepath.Base(absolute)), nil
}

// Close waits for in-flight writes, prevents further updates and releases
// ownership. The sidecar must stay in place so contenders lock the same inode.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.owner == nil {
		return nil
	}
	return s.owner.Close()
}
