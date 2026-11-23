package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	return runArgs(os.Args[1:])
}

func runArgs(args []string) error {
	flags := flag.NewFlagSet("campusjudge", flag.ContinueOnError)
	addr := flags.String("addr", "127.0.0.1:8080", "HTTP listen address")
	path := flags.String("data", "data/state.json", "local JSON data file (when CAMPUS_DATABASE_URL is unset)")
	importJSON := flags.String("import-json", "", "import an existing JSON state into empty PostgreSQL storage, then exit")
	demo := flags.Bool("demo", false, "use public teacher demo password on first initialization (loopback only)")
	judging := flags.String("judge", "docker", "judging mode: docker or manual")
	judgeImage := flags.String("judge-image", "campusjudge-runner:local", "locally built Docker runner image")
	proxyList := flags.String("trusted-proxies", "", "comma-separated proxy CIDRs allowed to supply X-Forwarded-For and X-Forwarded-Proto")
	healthcheck := flags.Bool("healthcheck", false, "check the local HTTP server and exit without opening storage")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *healthcheck {
		return checkHealth(*addr)
	}
	dataExplicit := false
	flags.Visit(func(option *flag.Flag) {
		if option.Name == "data" {
			dataExplicit = true
		}
	})
	trustedProxies, err := parseTrustedProxies(*proxyList)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, serverConfig{
		addr: *addr, dataPath: *path, demo: *demo, judging: *judging,
		judgeImage: *judgeImage, trustedProxies: trustedProxies,
		databaseURL: os.Getenv("CAMPUS_DATABASE_URL"), importJSON: *importJSON,
		dataExplicit: dataExplicit,
	})
}

type serverConfig struct {
	addr           string
	dataPath       string
	databaseURL    string
	importJSON     string
	dataExplicit   bool
	demo           bool
	judging        string
	judgeImage     string
	trustedProxies []netip.Prefix
}

func validateStorageConfig(config serverConfig) error {
	if config.databaseURL != "" && config.dataExplicit {
		return errors.New("-data selects JSON storage and cannot be used with CAMPUS_DATABASE_URL")
	}
	if config.importJSON != "" && config.databaseURL == "" {
		return errors.New("-import-json requires CAMPUS_DATABASE_URL")
	}
	return nil
}

func openData(ctx context.Context, config serverConfig) (*store.Store, error) {
	if config.databaseURL != "" {
		return store.OpenPostgres(ctx, config.databaseURL)
	}
	return store.OpenExclusive(config.dataPath)
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
	ctx, cancelService := context.WithCancel(ctx)
	defer cancelService()
	if err := validateStorageConfig(config); err != nil {
		return err
	}
	if config.judging != "docker" && config.judging != "manual" {
		return errors.New("-judge must be docker or manual")
	}
	s, err := openData(ctx, config)
	if err != nil {
		return fmt.Errorf("open data: %w", err)
	}
	defer func() {
		if closeErr := s.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("release data ownership: %w", closeErr))
		}
	}()
	if config.importJSON != "" {
		if err := s.ImportJSON(config.importJSON); err != nil {
			return fmt.Errorf("import JSON data: %w", err)
		}
		slog.Info("CampusJudge JSON data imported into PostgreSQL")
		return nil
	}
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
	teacherPassword := os.Getenv("CAMPUS_TEACHER_PASSWORD")
	bootstrap, err := app.Initialize(s, app.BootstrapOptions{
		Demo:            config.demo,
		TeacherPassword: teacherPassword,
		Now:             time.Now(),
	})
	if err != nil {
		return err
	}
	if bootstrap.Created {
		printBootstrap(os.Stdout, bootstrap, teacherPassword == "" && !config.demo)
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
	if s.Failure() != nil {
		go monitorStorage(ctx, s)
	}
	slog.Info("CampusJudge ready", "url", "http://"+listener.Addr().String())
	go func() { failure <- server.Serve(listener) }()
	select {
	case err := <-failure:
		cancelService()
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case storageErr := <-s.Failure():
		// The old snapshot cannot be reused after losing database ownership.
		// Exit so the container's restart policy reloads committed state.
		cancelService()
		return errors.Join(storageErr, shutdownHTTP(server))
	case <-ctx.Done():
		return shutdownHTTP(server)
	}
}

func monitorStorage(ctx context.Context, s *store.Store) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Check(ctx); err != nil {
				return
			}
		}
	}
}

func shutdownHTTP(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return errors.Join(err, server.Close())
	}
	return nil
}

func printBootstrap(output io.Writer, bootstrap app.Bootstrap, revealPassword bool) {
	fmt.Fprintf(output, "Первый запуск CampusJudge\nЛогин преподавателя: %s\n", bootstrap.Username)
	if revealPassword {
		fmt.Fprintf(output, "Пароль: %s\nСохраните пароль: при следующем запуске он не выводится.\n", bootstrap.Password)
	} else {
		fmt.Fprintln(output, "Пароль задан при настройке и не выводится в журнал.")
	}
	fmt.Fprintf(output, "Код тестовой группы: %s\n", bootstrap.InviteCode)
}
