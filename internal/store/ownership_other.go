//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package store

import (
	"errors"
	"os"
)

func lockStateFile(string) (*os.File, error) {
	return nil, errors.New("state ownership locking requires Linux, macOS or BSD")
}
