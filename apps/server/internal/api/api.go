// Package api owns every route in openapi/snarvei.yaml plus the hand-routed
// binary endpoints. It never reads the environment or constructs a
// dependency: cmd/snarvei hands it a Deps.
package api

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5/pgxpool"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/api/respond"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/clientip"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/email"
	"github.com/refsdal/snarvei/server/internal/ratelimit"
	"github.com/refsdal/snarvei/server/internal/storage"
)

// specYAML is the committed copy of openapi/snarvei.yaml (see generate.go).
//
//go:embed snarvei.yaml
var specYAML []byte

// Deps is everything the handlers need.
type Deps struct {
	Pool    *pgxpool.Pool
	Q       *dbgen.Queries
	Auth    auth.Service
	Storage storage.Storage
	Email   email.Sender
	// Mail is set only when TestHooks is on: the same Recording the Email
	// field points at, exposed at GET /api/_test/mail for Playwright.
	Mail      *email.Recording
	RateLimit ratelimit.Store
	Hasher    *clientip.Hasher
	Log       *slog.Logger

	AppURL           string
	AppName          string
	OpenSignup       bool
	Version          string
	TrustedProxyHops int
	TestHooks        bool
}

func (d Deps) log() *slog.Logger {
	if d.Log == nil {
		return slog.Default()
	}
	return d.Log
}

func loadSpec() *openapi3.T {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(specYAML)
	if err != nil {
		panic(fmt.Sprintf("api: parse embedded spec: %v", err))
	}
	if err := spec.Validate(loader.Context); err != nil {
		panic(fmt.Sprintf("api: embedded spec is invalid: %v", err))
	}
	return spec
}

// withSpecValidation rejects requests that do not match the spec. Unmatched
// routes fall through to next (the JSON 404); matched-but-invalid ones get
// 400 VALIDATION_FAILED.
func withSpecValidation(spec *openapi3.T, next http.Handler) http.Handler {
	validate := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers: true,
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
			if opts.MatchedRoute == nil {
				next.ServeHTTP(w, r)
				return
			}
			respond.JSON(w, http.StatusBadRequest, respond.Envelope{
				Code: "VALIDATION_FAILED", Message: "Invalid request",
				Details: map[string]any{"reason": err.Error()},
			})
		},
	})
	return validate(next)
}

func handleNotFound(w http.ResponseWriter, _ *http.Request) {
	respond.Error(w, http.StatusNotFound, "NOT_FOUND", "Not found")
}

func requestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	slog.Default().Error("invalid request", "event", "request.invalid", "method", r.Method, "path", r.URL.Path, "error", err.Error())
	respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Invalid request")
}

func (d Deps) responseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	err = classify(err)
	var he *httpError
	if errors.As(err, &he) {
		respond.Error(w, he.status, he.code, he.message)
		return
	}
	d.log().Error("unhandled error", "event", "request.error", "method", r.Method, "path", r.URL.Path, "error", err.Error())
	respond.Error(w, http.StatusInternalServerError, "INTERNAL", "Internal error")
}

// noStore marks probe responses uncacheable.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// NewHandler builds the handler for every server-owned path.
func NewHandler(d Deps) http.Handler {
	spec := loadSpec()
	assertTierCoverage(spec)
	if d.Auth == nil || d.Q == nil || d.RateLimit == nil || d.Hasher == nil {
		panic("api: NewHandler needs Auth, Q, RateLimit and Hasher")
	}

	mux := http.NewServeMux()
	mux.Handle(auth.BasePath+"/", d.Auth.Handler())
	d.mountImageRoutes(mux)
	if d.TestHooks {
		d.mountTestHooks(mux)
	}
	mux.Handle("/", withSpecValidation(spec, http.HandlerFunc(handleNotFound)))

	strict := gen.NewStrictHandlerWithOptions(d, []gen.StrictMiddlewareFunc{d.tierMiddleware()}, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler,
		ResponseErrorHandlerFunc: d.responseErrorHandler,
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter: mux,
		Middlewares: []gen.MiddlewareFunc{
			func(next http.Handler) http.Handler { return withSpecValidation(spec, next) },
		},
	})

	return middleware.TrustedProxy(d.TrustedProxyHops)(noStore(mux))
}
