package store

import (
	"strings"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestPostgresRowsRejectActualNULAndPreserveLiteralEscapes(t *testing.T) {
	for _, text := range []string{"actual\x00NUL", "nested\x00value"} {
		state := model.State{Problems: []model.Problem{{
			ID: "problem", Tests: []model.TestCase{{Input: text, Output: "answer"}},
		}}}
		_, err := postgresRows(state)
		if err == nil || !strings.Contains(err.Error(), "PostgreSQL problems record contains unsupported U+0000") {
			t.Fatalf("actual NUL error = %v", err)
		}
		if strings.Contains(err.Error(), text) {
			t.Fatal("diagnostic exposed record contents")
		}
	}
	for _, text := range []string{`literal\0`, `literal\u0000`, `literal\\u0000`} {
		state := model.State{Problems: []model.Problem{{
			ID: "problem", Tests: []model.TestCase{{Input: text, Output: "answer"}},
		}}}
		if _, err := postgresRows(state); err != nil {
			t.Fatalf("literal escape %q rejected: %v", text, err)
		}
	}
	state := model.State{Works: []model.Work{{ID: "work", ProblemRevisions: map[string]int{"bad\x00key": 1}}}}
	if _, err := postgresRows(state); err == nil || !strings.Contains(err.Error(), "unsupported U+0000") {
		t.Fatalf("NUL in JSON object key error = %v", err)
	}
}
