package store

import (
	"errors"
	"fmt"
	"os"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

// ImportJSON copies a stopped JSON installation into an empty PostgreSQL store.
// The source is locked against the legacy server and is never modified. The
// destination publishes all entities together, using its normal transaction.
func (s *Store) ImportJSON(path string) error {
	if s.backend == nil {
		return errors.New("JSON import requires a PostgreSQL destination")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect import source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("import source must be a regular JSON file")
	}
	source, err := OpenExclusive(path)
	if err != nil {
		return fmt.Errorf("open import source: %w", err)
	}
	defer source.Close()
	// Open normally permits a missing file. Recheck after ownership is acquired
	// so a removed source cannot silently become a successful empty import.
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("inspect locked import source: %w", err)
	}
	imported := source.Snapshot()
	if emptyClassroom(imported) {
		return errors.New("import source contains no classroom data")
	}
	return s.Update(func(target *model.State) error {
		if !emptyClassroom(*target) {
			return errors.New("PostgreSQL destination is not empty; import will not replace existing data")
		}
		*target = clone(imported)
		return nil
	})
}

func emptyClassroom(state model.State) bool {
	return !state.Demo && len(state.Users) == 0 && len(state.Groups) == 0 &&
		len(state.Topics) == 0 && len(state.Problems) == 0 && len(state.Works) == 0 &&
		len(state.Submissions) == 0 && len(state.Sessions) == 0
}
