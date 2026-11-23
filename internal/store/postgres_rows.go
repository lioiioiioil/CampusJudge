package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type postgresTable struct {
	name    string
	keys    []string
	columns []string
	rows    map[string]string
}

func postgresRows(state model.State) ([]postgresTable, error) {
	tables := []postgresTable{
		{name: "groups", keys: []string{"id"}, columns: strings.Fields("id position name invite_code")},
		{name: "users", keys: []string{"id"}, columns: strings.Fields("id position name username role group_id password_hash")},
		{name: "topics", keys: []string{"id"}, columns: strings.Fields("id position title description lesson_url")},
		{name: "problems", keys: []string{"id"}, columns: strings.Fields("id position title statement input_format output_format time_limit_ms memory_limit_mb samples tests test_revisions_set")},
		{name: "test_revisions", keys: []string{"problem_id", "number"}, columns: strings.Fields("problem_id number position samples tests created_at")},
		{name: "works", keys: []string{"id"}, columns: strings.Fields("id position title description kind topic_id status starts_at ends_at created_at group_ids_set problem_ids_set problem_revisions")},
		{name: "work_groups", keys: []string{"work_id", "position"}, columns: strings.Fields("work_id position group_id")},
		{name: "work_problems", keys: []string{"work_id", "position"}, columns: strings.Fields("work_id position problem_id")},
		{name: "submissions", keys: []string{"id"}, columns: strings.Fields("id position work_id problem_id user_id language source verdict feedback created_at judged_at reviewer_id judging test_revision past_judgments")},
		{name: "sessions", keys: []string{"token_hash"}, columns: strings.Fields("token_hash position user_id expires_at")},
	}
	for i := range tables {
		tables[i].rows = map[string]string{}
	}
	add := func(index int, item any, position int, extra map[string]any) error {
		return tables[index].add(item, position, extra)
	}
	for i, item := range state.Groups {
		if err := add(0, item, i, nil); err != nil {
			return nil, err
		}
	}
	for i, item := range state.Users {
		if err := add(1, item, i, nil); err != nil {
			return nil, err
		}
	}
	for i, item := range state.Topics {
		if err := add(2, item, i, nil); err != nil {
			return nil, err
		}
	}
	for i, item := range state.Problems {
		if err := add(3, item, i, map[string]any{"test_revisions_set": item.TestRevisions != nil, "tests": item.Tests}); err != nil {
			return nil, err
		}
		for j, revision := range item.TestRevisions {
			if err := add(4, revision, j, map[string]any{"problem_id": item.ID, "tests": revision.Tests}); err != nil {
				return nil, err
			}
		}
	}
	for i, item := range state.Works {
		if err := add(5, item, i, map[string]any{"group_ids_set": item.GroupIDs != nil, "problem_ids_set": item.ProblemIDs != nil, "problem_revisions": item.ProblemRevisions}); err != nil {
			return nil, err
		}
		for j, id := range item.GroupIDs {
			if err := add(6, map[string]any{"work_id": item.ID, "group_id": id}, j, nil); err != nil {
				return nil, err
			}
		}
		for j, id := range item.ProblemIDs {
			if err := add(7, map[string]any{"work_id": item.ID, "problem_id": id}, j, nil); err != nil {
				return nil, err
			}
		}
	}
	for i, item := range state.Submissions {
		if err := add(8, item, i, map[string]any{"test_revision": item.TestRevision, "past_judgments": item.PastJudgments}); err != nil {
			return nil, err
		}
	}
	for i, item := range state.Sessions {
		if err := add(9, item, i, nil); err != nil {
			return nil, err
		}
	}
	return tables, nil
}

func (t *postgresTable) add(item any, position int, extra map[string]any) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode PostgreSQL %s record", t.name)
	}
	// PostgreSQL text and JSONB cannot represent U+0000. Inspect decoded string
	// values, so literal source escapes such as `\0` and `\u0000` remain valid.
	var textValues any
	if err := json.Unmarshal(data, &textValues); err != nil {
		return fmt.Errorf("encode PostgreSQL %s record", t.name)
	}
	if containsPostgresNUL(textValues) {
		return fmt.Errorf("PostgreSQL %s record contains unsupported U+0000", t.name)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("encode PostgreSQL %s record", t.name)
	}
	fields["position"], _ = json.Marshal(position)
	for name, value := range extra {
		fields[name], _ = json.Marshal(value)
	}
	// Empty optional references are NULL rather than references to the empty ID.
	for _, name := range []string{"topic_id", "reviewer_id", "group_id", "invite_code"} {
		if string(fields[name]) == `""` {
			fields[name] = json.RawMessage("null")
		}
	}
	stored := make(map[string]json.RawMessage, len(t.columns))
	for _, name := range t.columns {
		value := fields[name]
		if value == nil {
			value = json.RawMessage("null")
		}
		stored[name] = value
	}
	keyParts := make([]json.RawMessage, len(t.keys))
	for i, name := range t.keys {
		keyParts[i] = stored[name]
	}
	keyJSON, _ := json.Marshal(keyParts)
	key := string(keyJSON)
	if _, exists := t.rows[key]; exists {
		return fmt.Errorf("duplicate PostgreSQL %s record key", t.name)
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("encode PostgreSQL %s record", t.name)
	}
	t.rows[key] = string(encoded)
	return nil
}

func containsPostgresNUL(value any) bool {
	switch item := value.(type) {
	case string:
		return strings.ContainsRune(item, '\x00')
	case []any:
		for _, child := range item {
			if containsPostgresNUL(child) {
				return true
			}
		}
	case map[string]any:
		for key, child := range item {
			if strings.ContainsRune(key, '\x00') || containsPostgresNUL(child) {
				return true
			}
		}
	}
	return false
}

func (t postgresTable) deleteRemoved(ctx context.Context, tx pgx.Tx, previous postgresTable) error {
	conditions := make([]string, len(t.keys))
	for i, key := range t.keys {
		conditions[i] = "target_row." + key + "=old." + key
	}
	query := "DELETE FROM campusjudge." + t.name + " target_row USING jsonb_populate_record(NULL::campusjudge." + t.name + ", $1::jsonb) old WHERE " + strings.Join(conditions, " AND ")
	for _, key := range sortedPostgresKeys(previous.rows) {
		if _, exists := t.rows[key]; exists {
			continue
		}
		if _, err := tx.Exec(ctx, query, previous.rows[key]); err != nil {
			return postgresError("delete "+t.name, err)
		}
	}
	return nil
}

func (t postgresTable) upsertChanged(ctx context.Context, tx pgx.Tx, previous postgresTable) error {
	assignments := []string{}
	for _, name := range t.columns {
		if !slices.Contains(t.keys, name) {
			assignments = append(assignments, name+"=EXCLUDED."+name)
		}
	}
	// jsonb_populate_record maps the parameter into real typed relational columns;
	// the JSON parameter is only transport, not a stored whole-state document.
	// https://www.postgresql.org/docs/17/functions-json.html#FUNCTIONS-JSON-PROCESSING-TABLE
	query := "INSERT INTO campusjudge." + t.name + " SELECT * FROM jsonb_populate_record(NULL::campusjudge." + t.name + ", $1::jsonb) ON CONFLICT (" + strings.Join(t.keys, ",") + ") DO UPDATE SET " + strings.Join(assignments, ",")
	for _, key := range sortedPostgresKeys(t.rows) {
		if previous.rows[key] == t.rows[key] {
			continue
		}
		if _, err := tx.Exec(ctx, query, t.rows[key]); err != nil {
			return postgresError("save "+t.name, err)
		}
	}
	return nil
}

func sortedPostgresKeys(rows map[string]string) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
