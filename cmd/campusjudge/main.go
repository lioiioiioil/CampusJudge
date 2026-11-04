package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/app"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("CampusJudge stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	path := flag.String("data", "data/state.json", "local JSON data file")
	demo := flag.Bool("demo", false, "use public teacher demo password on first initialization (loopback only)")
	flag.Parse()
	s, err := store.Open(*path)
	if err != nil {
		return fmt.Errorf("open data: %w", err)
	}
	// A public demo password must not be exposed by accidentally changing -addr.
	if *demo || s.Snapshot().Demo {
		host, _, err := net.SplitHostPort(*addr)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("demo credentials are restricted to a loopback address")
		}
	}
	bootstrap, err := app.Initialize(s, *demo)
	if err != nil {
		return err
	}
	if bootstrap.Created {
		fmt.Printf("Первый запуск CampusJudge\nЛогин преподавателя: %s\nПароль: %s\nКод тестовой группы: %s\nСохраните пароль: при следующем запуске он не выводится.\n", bootstrap.Username, bootstrap.Password, bootstrap.InviteCode)
	}
	server := &http.Server{Addr: *addr, Handler: app.New(s).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failure := make(chan error, 1)
	go func() { slog.Info("CampusJudge ready", "url", "http://"+*addr); failure <- server.ListenAndServe() }()
	select {
	case err := <-failure:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
