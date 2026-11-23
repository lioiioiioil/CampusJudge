package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

//go:embed migrations/001_initial.sql
var initialPostgresSchema string

const postgresSchemaVersion = 1
const postgresOperationTimeout = 15 * time.Second
const postgresOwnerLock int64 = 0x43414d5055534a

var ErrDatabaseInUse = errors.New("PostgreSQL database is already in use by another CampusJudge instance")

type postgresBackend struct {
	conn    *pgx.Conn
	rows    []postgresTable
	failed  bool
	failure chan error
}

// OpenPostgres holds one session-level advisory lock for the lifetime of the
// Store. Keeping the lock and writes on the same connection makes a lost
// connection fail closed, rather than reconnecting with a stale memory snapshot.
// https://www.postgresql.org/docs/17/explicit-locking.html#ADVISORY-LOCKS
func OpenPostgres(ctx context.Context, dsn string) (_ *Store, err error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("PostgreSQL connection string is empty")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	config.RuntimeParams["application_name"] = "CampusJudge"
	config.RuntimeParams["timezone"] = "UTC"
	config.RuntimeParams["statement_timeout"] = "15000"
	config.RuntimeParams["lock_timeout"] = "5000"
	ctx, cancel := context.WithTimeout(ctx, postgresOperationTimeout)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, postgresError("connect", err)
	}
	backend := &postgresBackend{conn: conn, failure: make(chan error, 1)}
	defer func() {
		if err != nil {
			_ = backend.Close()
		}
	}()
	var acquired bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1::bigint)", postgresOwnerLock).Scan(&acquired); err != nil {
		return nil, postgresError("acquire ownership", err)
	}
	if !acquired {
		return nil, ErrDatabaseInUse
	}
	if err = backend.migrate(ctx); err != nil {
		return nil, err
	}
	state, err := backend.load(ctx)
	if err != nil {
		return nil, err
	}
	backend.rows, err = postgresRows(state)
	if err != nil {
		return nil, err
	}
	return &Store{state: state, backend: backend}, nil
}

func (p *postgresBackend) migrate(ctx context.Context) error {
	tx, err := p.conn.Begin(ctx)
	if err != nil {
		return postgresError("begin schema migration", err)
	}
	defer rollbackPostgres(tx)
	if _, err := tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS campusjudge"); err != nil {
		return postgresError("create schema", err)
	}
	var existing *string
	if err := tx.QueryRow(ctx, "SELECT to_regclass('campusjudge.metadata')::text").Scan(&existing); err != nil {
		return postgresError("inspect schema", err)
	}
	if existing == nil {
		if _, err := tx.Exec(ctx, initialPostgresSchema); err != nil {
			return postgresError("initialize schema", err)
		}
	}
	var schemaVersion, stateVersion, count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM campusjudge.metadata").Scan(&count); err != nil {
		return postgresError("read schema version", err)
	}
	if count != 1 {
		return errors.New("PostgreSQL metadata must contain exactly one row")
	}
	if err := tx.QueryRow(ctx, "SELECT schema_version, state_version FROM campusjudge.metadata WHERE singleton").Scan(&schemaVersion, &stateVersion); err != nil {
		return postgresError("read schema version", err)
	}
	if schemaVersion != postgresSchemaVersion {
		return fmt.Errorf("unsupported PostgreSQL schema version %d (expected %d)", schemaVersion, postgresSchemaVersion)
	}
	if stateVersion != currentVersion {
		return fmt.Errorf("unsupported state version %d (expected %d)", stateVersion, currentVersion)
	}
	if err := tx.Commit(ctx); err != nil {
		return postgresError("commit schema migration", err)
	}
	return nil
}

func (p *postgresBackend) Save(state *model.State) (err error) {
	defer func() {
		if err != nil && (p.failed || p.conn.IsClosed()) {
			p.reportFailure()
		}
	}()
	if p.failed || p.conn.IsClosed() {
		return errors.New("PostgreSQL connection lost; restart CampusJudge to reload committed data")
	}
	normalizePostgresTimes(state)
	next, err := postgresRows(*state)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	tx, err := p.conn.Begin(ctx)
	if err != nil {
		return postgresError("begin state transaction", err)
	}
	defer rollbackPostgres(tx)
	for i := len(next) - 1; i >= 0; i-- {
		if err := next[i].deleteRemoved(ctx, tx, p.rows[i]); err != nil {
			return err
		}
	}
	for i := range next {
		if err := next[i].upsertChanged(ctx, tx, p.rows[i]); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE campusjudge.metadata SET state_version=$1, demo=$2 WHERE singleton", state.Version, state.Demo); err != nil {
		return postgresError("save metadata", err)
	}
	if err := tx.Commit(ctx); err != nil {
		// A server error explicitly rejects COMMIT. A transport error may have
		// arrived after a successful COMMIT; prevent retries from stale memory.
		var serverError *pgconn.PgError
		if !errors.As(err, &serverError) && !errors.Is(err, pgx.ErrTxCommitRollback) {
			p.failed = true
			_ = p.Close()
		}
		return postgresError("commit state transaction", err)
	}
	p.rows = next
	return nil
}

func (p *postgresBackend) Failure() <-chan error { return p.failure }

func (p *postgresBackend) reportFailure() {
	p.failed = true
	select {
	case p.failure <- errors.New("PostgreSQL connection lost; restart CampusJudge to reload committed data"):
	default:
	}
}

func (p *postgresBackend) Check(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if p.failed || p.conn.IsClosed() {
		p.reportFailure()
		return errors.New("PostgreSQL connection lost; restart CampusJudge to reload committed data")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.conn.Ping(probeCtx); err != nil {
		// Shutdown cancellation is not a reason to report an unhealthy service.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.reportFailure()
		_ = p.Close()
		return postgresError("check connection", err)
	}
	return nil
}

func (p *postgresBackend) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.conn.Close(ctx); err != nil {
		return postgresError("close connection", err)
	}
	return nil
}

func rollbackPostgres(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// Driver errors can contain passwords, query values, or connection URLs. Expose
// only a safe operation and SQLSTATE; never include their raw Error() strings.
func postgresError(operation string, err error) error {
	var serverError *pgconn.PgError
	if errors.As(err, &serverError) {
		return fmt.Errorf("PostgreSQL %s failed (SQLSTATE %s)", operation, serverError.Code)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("PostgreSQL %s timed out", operation)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("PostgreSQL %s canceled", operation)
	}
	return fmt.Errorf("PostgreSQL %s failed", operation)
}

// PostgreSQL timestamps have microsecond resolution. Canonicalize before both
// persistence and in-memory publication, including timestamps nested in JSONB.
// https://www.postgresql.org/docs/17/datatype-datetime.html
func normalizePostgresTimes(state *model.State) {
	canonical := func(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }
	optional := func(t *time.Time) {
		if t != nil {
			*t = canonical(*t)
		}
	}
	for i := range state.Problems {
		for j := range state.Problems[i].TestRevisions {
			r := &state.Problems[i].TestRevisions[j]
			r.CreatedAt = canonical(r.CreatedAt)
		}
	}
	for i := range state.Works {
		w := &state.Works[i]
		w.CreatedAt = canonical(w.CreatedAt)
		optional(w.StartsAt)
		optional(w.EndsAt)
	}
	for i := range state.Submissions {
		s := &state.Submissions[i]
		s.CreatedAt = canonical(s.CreatedAt)
		optional(s.JudgedAt)
		for j := range s.PastJudgments {
			s.PastJudgments[j].ReplacedAt = canonical(s.PastJudgments[j].ReplacedAt)
		}
	}
	for i := range state.Sessions {
		state.Sessions[i].ExpiresAt = canonical(state.Sessions[i].ExpiresAt)
	}
}

func postgresLoadRows[T any](ctx context.Context, conn *pgx.Conn, table, order string) ([]T, error) {
	rows, err := conn.Query(ctx, "SELECT to_jsonb(t) FROM campusjudge."+table+" t ORDER BY "+order)
	if err != nil {
		return nil, postgresError("read "+table, err)
	}
	defer rows.Close()
	result := []T{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, postgresError("scan "+table, err)
		}
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("invalid stored PostgreSQL %s record", table)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, postgresError("read "+table, err)
	}
	return result, nil
}

func (p *postgresBackend) load(ctx context.Context) (model.State, error) {
	state := model.State{}
	if err := p.conn.QueryRow(ctx, "SELECT state_version, demo FROM campusjudge.metadata WHERE singleton").Scan(&state.Version, &state.Demo); err != nil {
		return state, postgresError("read metadata", err)
	}
	var err error
	if state.Groups, err = postgresLoadRows[model.Group](ctx, p.conn, "groups", "position"); err != nil {
		return state, err
	}
	if state.Users, err = postgresLoadRows[model.User](ctx, p.conn, "users", "position"); err != nil {
		return state, err
	}
	if state.Topics, err = postgresLoadRows[model.Topic](ctx, p.conn, "topics", "position"); err != nil {
		return state, err
	}
	if state.Problems, err = postgresLoadRows[model.Problem](ctx, p.conn, "problems", "position"); err != nil {
		return state, err
	}
	if state.Works, err = postgresLoadRows[model.Work](ctx, p.conn, "works", "position"); err != nil {
		return state, err
	}
	if state.Submissions, err = postgresLoadRows[model.Submission](ctx, p.conn, "submissions", "position"); err != nil {
		return state, err
	}
	if state.Sessions, err = postgresLoadRows[model.Session](ctx, p.conn, "sessions", "position"); err != nil {
		return state, err
	}
	problemIndices := map[string]int{}
	for i := range state.Problems {
		problemIndices[state.Problems[i].ID] = i
	}
	type problemShape struct {
		ID  string `json:"id"`
		Set bool   `json:"test_revisions_set"`
	}
	shapes, err := postgresLoadRows[problemShape](ctx, p.conn, "problems", "position")
	if err != nil {
		return state, err
	}
	for _, shape := range shapes {
		if shape.Set {
			state.Problems[problemIndices[shape.ID]].TestRevisions = []model.TestRevision{}
		}
	}
	type revision struct {
		model.TestRevision
		ProblemID string `json:"problem_id"`
	}
	revisions, err := postgresLoadRows[revision](ctx, p.conn, "test_revisions", "problem_id, position")
	if err != nil {
		return state, err
	}
	for _, item := range revisions {
		index := problemIndices[item.ProblemID]
		state.Problems[index].TestRevisions = append(state.Problems[index].TestRevisions, item.TestRevision)
	}
	workIndices := map[string]int{}
	for i := range state.Works {
		workIndices[state.Works[i].ID] = i
	}
	type workShape struct {
		ID       string `json:"id"`
		Groups   bool   `json:"group_ids_set"`
		Problems bool   `json:"problem_ids_set"`
	}
	workShapes, err := postgresLoadRows[workShape](ctx, p.conn, "works", "position")
	if err != nil {
		return state, err
	}
	for _, shape := range workShapes {
		index := workIndices[shape.ID]
		if shape.Groups {
			state.Works[index].GroupIDs = []string{}
		}
		if shape.Problems {
			state.Works[index].ProblemIDs = []string{}
		}
	}
	type groupLink struct {
		WorkID  string `json:"work_id"`
		GroupID string `json:"group_id"`
	}
	groups, err := postgresLoadRows[groupLink](ctx, p.conn, "work_groups", "work_id, position")
	if err != nil {
		return state, err
	}
	for _, item := range groups {
		index := workIndices[item.WorkID]
		state.Works[index].GroupIDs = append(state.Works[index].GroupIDs, item.GroupID)
	}
	type problemLink struct {
		WorkID    string `json:"work_id"`
		ProblemID string `json:"problem_id"`
	}
	problems, err := postgresLoadRows[problemLink](ctx, p.conn, "work_problems", "work_id, position")
	if err != nil {
		return state, err
	}
	for _, item := range problems {
		index := workIndices[item.WorkID]
		state.Works[index].ProblemIDs = append(state.Works[index].ProblemIDs, item.ProblemID)
	}
	normalize(&state)
	normalizePostgresTimes(&state)
	return state, nil
}
