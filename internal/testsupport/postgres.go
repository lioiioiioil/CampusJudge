// Package testsupport owns disposable resources used by integration tests.
package testsupport

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// PostgresURL creates a separate database on the explicitly configured test
// server. A CREATEDB role is required. It never resets an existing database.
func PostgresURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CAMPUSJUDGE_POSTGRES_TEST_URL")
	if dsn == "" {
		t.Skip("set CAMPUSJUDGE_POSTGRES_TEST_URL to a disposable PostgreSQL server")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("CAMPUSJUDGE_POSTGRES_TEST_URL must be a PostgreSQL URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("connect to disposable PostgreSQL server failed")
	}
	database := "campusjudge_test_" + strings.ToLower(rand.Text())
	identifier := pgx.Identifier{database}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create test database (CREATEDB required): %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		_ = admin.Close(cleanupCtx)
	})
	parsed.Path, parsed.RawPath = "/"+database, ""
	// Both aliases override a URL path in pgx. Remove them so tests cannot
	// accidentally write into the administrative database supplied by the user.
	query := parsed.Query()
	query.Del("dbname")
	query.Del("database")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
