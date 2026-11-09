package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/app"
	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/store"
	"github.com/lioiioiioil/CampusJudge/web"
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
	judging := flag.String("judge", "docker", "judging mode: docker or manual")
	judgeImage := flag.String("judge-image", "campusjudge-runner:local", "locally built Docker runner image")
	proxyList := flag.String("trusted-proxies", "", "comma-separated proxy CIDRs allowed to supply X-Forwarded-For and X-Forwarded-Proto")
	flag.Parse()
	trustedProxies, err := parseTrustedProxies(*proxyList)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, serverConfig{
		addr: *addr, dataPath: *path, demo: *demo, judging: *judging,
		judgeImage: *judgeImage, trustedProxies: trustedProxies,
	})
}

type serverConfig struct {
	addr           string
	dataPath       string
	demo           bool
	judging        string
	judgeImage     string
	trustedProxies []netip.Prefix
}

func parseTrustedProxies(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var prefixes []netip.Prefix
	for _, raw := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("-trusted-proxies requires explicit IPv4 or IPv6 CIDRs, got %q", raw)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func serve(ctx context.Context, config serverConfig) (err error) {
	if config.judging != "docker" && config.judging != "manual" {
		return errors.New("-judge must be docker or manual")
	}
	s, err := store.OpenExclusive(config.dataPath)
	if err != nil {
		return fmt.Errorf("open data: %w", err)
	}
	defer func() {
		if closeErr := s.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("release data ownership: %w", closeErr))
		}
	}()
	// A public demo password must not be exposed by accidentally changing -addr.
	if config.demo || s.Snapshot().Demo {
		if len(config.trustedProxies) > 0 {
			return errors.New("trusted proxies require a non-demo state")
		}
		host, _, err := net.SplitHostPort(config.addr)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("demo credentials are restricted to a loopback address")
		}
	}
	listener, err := net.Listen("tcp", config.addr)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}
	defer listener.Close()

	var runner judge.Runner
	if config.judging == "docker" {
		docker, err := judge.NewDocker(judge.DockerOptions{Binary: "docker", Image: config.judgeImage})
		if err != nil {
			return err
		}
		checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = docker.Check(checkCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("automatic judge unavailable: %w; see docs/automatic-judge.md, or choose -judge manual", err)
		}
		runner = docker
	}
	bootstrap, err := app.Initialize(s, app.BootstrapOptions{
		Demo:            config.demo,
		TeacherPassword: os.Getenv("CAMPUS_TEACHER_PASSWORD"),
		Now:             time.Now(),
	})
	if err != nil {
		return err
	}
	if bootstrap.Created {
		fmt.Printf("Первый запуск CampusJudge\nЛогин преподавателя: %s\nПароль: %s\nКод тестовой группы: %s\nСохраните пароль: при следующем запуске он не выводится.\n", bootstrap.Username, bootstrap.Password, bootstrap.InviteCode)
	}
	mux := http.NewServeMux()
	application := app.New(s, config.trustedProxies...)
	if runner != nil {
		judgeCtx, cancel := context.WithCancel(ctx)
		if err := application.StartJudge(judgeCtx, runner); err != nil {
			cancel()
			return fmt.Errorf("start judge: %w", err)
		}
		defer func() { cancel(); application.WaitJudge() }()
	} else if err := application.UseManualJudging(); err != nil {
		return err
	}
	api := application.Handler()
	mux.Handle("/api/", api)
	mux.Handle("/healthz", api)
	mux.Handle("/", web.Handler())
	server := &http.Server{
		Addr:              config.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	failure := make(chan error, 1)
	slog.Info("CampusJudge ready", "url", "http://"+listener.Addr().String())
	go func() { failure <- server.Serve(listener) }()
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
