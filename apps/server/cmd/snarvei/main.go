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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// The distroless image has no zoneinfo; compile it in.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	"github.com/refsdal/snarvei/server/internal/config"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/storage"
	"github.com/refsdal/snarvei/server/internal/web"
)

// version is injected with -ldflags "-X main.version=<tag or sha>".
var version = "dev"

const (
	shutdownTimeout   = 20 * time.Second
	readHeaderTimeout = 15 * time.Second
	idleTimeout       = 120 * time.Second
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

	pool, err := db.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Printf("startup failed: %v", err)
		return 1
	}
	defer pool.Close()

	deps, err := newDeps(pool, cfg)
	if err != nil {
		log.Printf("startup failed: %v", err)
		return 1
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           web.Handler(api.NewHandler(deps)),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	log.Printf("snarvei %s listening on http://0.0.0.0:%d", version, cfg.Port)
	log.Printf("  app url:  %s", cfg.AppURL)
	log.Printf("  storage:  %s", cfg.StorageDriver)
	if off := cfg.DisabledSubsystems(); len(off) > 0 {
		log.Printf("  disabled: %s", strings.Join(off, ", "))
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

// newDeps builds the api.Deps for a real process from cfg and an open pool.
// This is a minimal, provisional wiring: Task 11 (composition root) replaces
// it with buildDeps, JSON slog logging and the E2E test-hooks endpoint.
func newDeps(pool *pgxpool.Pool, cfg *config.Config) (api.Deps, error) {
	hasher := clientip.NewHasher(cfg.IPHashPepper, cfg.AuthSecret)
	q := dbgen.New(pool)

	var sender email.Sender
	if cfg.SMTPHost != "" {
		sender = email.NewSMTP(email.SMTPConfig{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort,
			Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
			From: cfg.EmailFrom,
		})
	} else {
		sender = email.NewNoop(nil)
	}

	svc, err := auth.New(auth.Config{
		AppURL:     cfg.AppURL,
		AppName:    cfg.AppName,
		Secret:     cfg.AuthSecret,
		OpenSignup: cfg.OpenSignup,
		Pool:       pool,
		ClientIP:   hasher.Extractor(cfg.TrustedProxyHops),
		Email:      sender,
	})
	if err != nil {
		return api.Deps{}, fmt.Errorf("auth: %w", err)
	}

	var store storage.Storage
	if cfg.StorageDriver == "s3" {
		store = storage.NewS3(storage.S3Config{
			Bucket: cfg.S3Bucket, Endpoint: cfg.S3Endpoint,
			AccessKeyID: cfg.S3AccessKeyID, SecretAccessKey: cfg.S3SecretAccessKey,
			Region: cfg.S3Region,
		})
	} else {
		store, err = storage.NewFS(cfg.StorageFSPath)
		if err != nil {
			return api.Deps{}, fmt.Errorf("storage: %w", err)
		}
	}

	return api.Deps{
		Pool:             pool,
		Q:                q,
		Auth:             svc,
		Storage:          store,
		Email:            sender,
		RateLimit:        ratelimit.NewPostgres(q),
		Hasher:           hasher,
		AppURL:           cfg.AppURL,
		AppName:          cfg.AppName,
		OpenSignup:       cfg.OpenSignup,
		Version:          version,
		TrustedProxyHops: cfg.TrustedProxyHops,
		TestHooks:        cfg.E2ETestHooks,
	}, nil
}
