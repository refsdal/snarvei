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

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/config"
	"github.com/refsdal/snarvei/server/internal/db"
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

	deps := api.Deps{Pool: pool, AppName: cfg.AppName, OpenSignup: cfg.OpenSignup, Version: version}

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
