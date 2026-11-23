package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lioiioiioil/CampusJudge/internal/testsupport"
)

func TestPostgresServiceExitsAfterIdleConnectionLoss(t *testing.T) {
	dsn := testsupport.PostgresURL(t)
	t.Setenv("CAMPUS_TEACHER_PASSWORD", "container-test-password")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		done <- serve(ctx, serverConfig{addr: addr, databaseURL: dsn, judging: "manual"})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(15 * time.Second):
			t.Error("service did not release its resources after cancellation")
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for checkHealth(addr) != nil {
		select {
		case err := <-done:
			t.Fatalf("service stopped before readiness: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("service startup timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
	adminCtx, adminCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer adminCancel()
	admin, err := pgx.Connect(adminCtx, dsn)
	if err != nil {
		t.Fatal("connect to test database")
	}
	defer admin.Close(adminCtx)
	rows, err := admin.Query(adminCtx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND application_name='CampusJudge' AND pid<>pg_backend_pid()")
	if err != nil {
		t.Fatal("terminate application database connection")
	}
	terminated := 0
	for rows.Next() {
		var ok bool
		if err := rows.Scan(&ok); err != nil || !ok {
			t.Fatal("application connection was not terminated")
		}
		terminated++
	}
	rows.Close()
	if rows.Err() != nil || terminated != 1 {
		t.Fatalf("terminated %d application connections", terminated)
	}
	// No user write occurs here: the periodic storage probe must discover the
	// lost owner and exit so a container restart can load current database state.
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "PostgreSQL") {
			t.Fatalf("service exit = %v, want a storage failure", err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("service kept serving its cached snapshot after losing PostgreSQL")
	}
}
