//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package store

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func lockStateFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state ownership lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect state ownership lock: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("state ownership lock must be a regular file")
	}
	if info.Size() != 0 {
		_ = file.Close()
		return nil, fmt.Errorf("state ownership lock must be empty: %s", path)
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: %s", ErrStateInUse, path)
		}
		return nil, fmt.Errorf("lock state file: %w", err)
	}
	return file, nil
}
