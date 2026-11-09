package domain

import "github.com/lioiioiioil/CampusJudge/internal/model"

// TestsAt treats the original problem fields as revision 1. The zero value on
// existing works and submissions therefore keeps their original test suite.
func TestsAt(problem model.Problem, number int) (model.TestRevision, bool) {
	if problem.ID == "" {
		return model.TestRevision{}, false
	}
	if number == 0 || number == 1 {
		return model.TestRevision{Number: 1, Samples: problem.Samples, Tests: problem.Tests}, true
	}
	for _, revision := range problem.TestRevisions {
		if revision.Number == number {
			return revision, true
		}
	}
	return model.TestRevision{}, false
}
