// Command snarvei is the container entrypoint and the composition root: the
// one place that reads the environment, opens the pool and hands
// collaborators to internal/api. It is also the dispatch table:
//
//	(none)                 migrate under an advisory lock, then serve
//	server                 HTTP only, never migrates (what replicas run)
//	migrate | migrations   apply migrations, exit 0/1
//	healthcheck            probe /healthz on this process, exit 0/1
//	anything else          usage, exit 2
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// The distroless image has no zoneinfo; compile it in.
	_ "time/tzdata"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/config"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/redirect"
	"github.com/refsdal/snarvei/server/internal/storage"
	"github.com/refsdal/snarvei/server/internal/web"
)

// version is injected with -ldflags "-X main.version=<tag or sha>".
var version = "dev"

const (
	shutdownTimeout   = 20 * time.Second
	readHeaderTimeout = 15 * time.Second
	idleTimeout       = 120 * time.Second
	clickDrainTimeout = 5 * time.Second
)

func main() { os.Exit(run(os.Args[1:])) }

type dispatchMode int

const (
	modeDefault dispatchMode = iota
	modeServer
	modeMigrate
	modeHealthcheck
	modeUnknown
)

type dispatch struct {
	mode dispatchMode
	raw  string
}

func parseArgs(args []string) dispatch {
	if len(args) == 0 || args[0] == "" {
		return dispatch{mode: modeDefault}
	}
	switch args[0] {
	case "server":
		return dispatch{mode: modeServer, raw: args[0]}
	case "migrate", "migrations":
		return dispatch{mode: modeMigrate, raw: args[0]}
	case "healthcheck":
		return dispatch{mode: modeHealthcheck, raw: args[0]}
	default:
		return dispatch{mode: modeUnknown, raw: args[0]}
	}
}

func run(args []string) int {
	d := parseArgs(args)
	switch d.mode {
	case modeHealthcheck:
		return healthcheckMode(portFromEnv())
	case modeUnknown:
		fmt.Fprintf(os.Stderr, "Unknown dispatch mode %q. Expected one of: server, migrate (or migrations), healthcheck, or no argument to migrate-then-serve.\n", d.raw)
		return 2
	case modeMigrate:
		return migrateMode()
	case modeServer:
		return serveMode(false)
	default:
		return serveMode(true)
	}
}

func portFromEnv() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "3000"
}

// healthcheckMode constructs nothing: a liveness probe must not fail because
// DATABASE_URL is wrong. The image has no shell or curl, so the binary probes itself.
func healthcheckMode(port string) int {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return 0
	}
	return 1
}

func migrateMode() int {
	cfg, err := config.FromOS()
	if err != nil {
		log.Printf("configuration error: %v", err)
		return 1
	}
	setupLogging(cfg.LogLevel)
	// Background, not signal-cancelled: aborting DDL halfway is worse than
	// being killed after the grace period.
	if err := db.ApplyMigrations(context.Background(), cfg.DatabaseURL, cfg.MigrationLockKey); err != nil {
		log.Printf("migration failed: %v", err)
		return 1
	}
	log.Print("migrations applied")
	return 0
}

func serveMode(migrate bool) int {
	cfg, err := config.FromOS()
	if err != nil {
		log.Printf("configuration error: %v", err)
		return 1
	}
	setupLogging(cfg.LogLevel)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)

	return serve(cfg, migrate, sig)
}

// serve runs the migrate-then-serve (or server-only) lifecycle for an
// already-loaded config: optionally migrate, open the pool, listen, and
// block until either the listener fails or sig delivers a shutdown signal.
// Split out of serveMode so tests can drive it against a testrig database
// and a real signal without touching the process environment or os.Args.
func serve(cfg *config.Config, migrate bool, sig <-chan os.Signal) int {
	if migrate {
		if err := db.ApplyMigrations(context.Background(), cfg.DatabaseURL, cfg.MigrationLockKey); err != nil {
			log.Printf("migration failed: %v", err)
			return 1
		}
	}

	deps, closeDeps, err := buildDeps(context.Background(), cfg)
	if err != nil {
		log.Printf("startup failed: %v", err)
		return 1
	}
	defer closeDeps()

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           web.Handler(api.NewHandler(deps)),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	slog.Info("snarvei listening", "version", version, "port", cfg.Port, "app_url", cfg.AppURL, "storage", cfg.StorageDriver, "disabled", cfg.DisabledSubsystems())
	if cfg.E2ETestHooks {
		slog.Info("test hooks enabled")
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server failed: %v", err)
			return 1
		}
		return 0
	case s := <-sig:
		log.Printf("%s received, shutting down", signalName(s))
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
		// Drain runs exactly once, after Shutdown returns: the recorder's
		// WaitGroup must not receive new Records once we start waiting on it.
		if !deps.Clicks.Drain(clickDrainTimeout) {
			slog.Warn("click recorder drain timed out", "event", "click.drain_timeout")
		}
		return 0
	}
}

func signalName(s os.Signal) string {
	switch s {
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGINT:
		return "SIGINT"
	default:
		return s.String()
	}
}

// setupLogging configures the process-wide slog default as JSON on stdout.
// Called once, right after config loads, in serveMode and migrateMode.
func setupLogging(level string) {
	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(strings.ToUpper(level)))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

// buildDeps assembles every collaborator from validated config. The only
// place in the program that constructs a client.
func buildDeps(ctx context.Context, cfg *config.Config) (api.Deps, func(), error) {
	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return api.Deps{}, nil, err
	}
	closePool := func() { pool.Close() }

	q := dbgen.New(pool)
	hasher := clientip.NewHasher(cfg.IPHashPepper, cfg.AuthSecret)
	sender, mailbox := buildEmail(cfg)
	authService, err := auth.New(auth.Config{
		AppURL: cfg.AppURL, AppName: cfg.AppName, Secret: cfg.AuthSecret, OpenSignup: cfg.OpenSignup,
		Pool: pool, ClientIP: hasher.Extractor(cfg.TrustedProxyHops), Email: sender, Log: slog.Default(),
	})
	if err != nil {
		closePool()
		return api.Deps{}, nil, err
	}
	store, err := buildStorage(cfg)
	if err != nil {
		closePool()
		return api.Deps{}, nil, err
	}
	return api.Deps{
		Pool: pool, Q: q, Auth: authService, Storage: store, Email: sender, Mail: mailbox,
		RateLimit: ratelimit.NewPostgres(q), Hasher: hasher, Clicks: redirect.NewRecorder(q, slog.Default()), Log: slog.Default(),
		AppURL: cfg.AppURL, AppName: cfg.AppName, OpenSignup: cfg.OpenSignup, Version: version,
		TrustedProxyHops: cfg.TrustedProxyHops, TestHooks: cfg.E2ETestHooks,
	}, closePool, nil
}

func buildStorage(cfg *config.Config) (storage.Storage, error) {
	switch cfg.StorageDriver {
	case "fs":
		return storage.NewFS(cfg.StorageFSPath)
	case "s3":
		return storage.NewS3(storage.S3Config{Bucket: cfg.S3Bucket, Endpoint: cfg.S3Endpoint, AccessKeyID: cfg.S3AccessKeyID, SecretAccessKey: cfg.S3SecretAccessKey, Region: cfg.S3Region}), nil
	}
	return nil, fmt.Errorf("unknown storage driver %q", cfg.StorageDriver)
}

// buildEmail picks SMTP when configured; with test hooks on, mail is captured
// in memory for the e2e suite instead (the hook endpoint reads it back).
func buildEmail(cfg *config.Config) (email.Sender, *email.Recording) {
	if cfg.E2ETestHooks {
		rec := email.NewRecording()
		return rec, rec
	}
	if cfg.EmailEnabled() {
		return email.NewSMTP(email.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.EmailFrom}), nil
	}
	return email.NewNoop(slog.Default()), nil
}
