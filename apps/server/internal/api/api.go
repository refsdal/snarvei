// Package api owns every route in openapi/snarvei.yaml. NewHandler validates
// requests against the embedded spec (kin-openapi), dispatches to the
// generated strict server, and answers a JSON 404 for anything the spec does
// not know. It never reads the environment or constructs a dependency: cmd/
// snarvei hands it a Deps.
package api

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/jackc/pgx/v5/pgxpool"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/respond"
)

// specYAML is the committed copy of openapi/snarvei.yaml (see generate.go).
//
//go:embed snarvei.yaml
var specYAML []byte

// Deps is everything the handlers need. Fields grow with each phase.
type Deps struct {
	Pool       *pgxpool.Pool
	AppName    string
	OpenSignup bool
	Version    string
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
	log.Printf("api: %s %s: invalid request: %v", r.Method, r.URL.Path, err)
	respond.Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Invalid request")
}

func responseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("api: %s %s: %v", r.Method, r.URL.Path, err)
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

// NewHandler builds the handler for every server-owned path. web.Handler
// wraps it and is what cmd/snarvei serves.
func NewHandler(d Deps) http.Handler {
	spec := loadSpec()
	mux := http.NewServeMux()

	// Least specific: any server-owned path the spec does not know answers a
	// JSON 404 (web.Handler routes /l/, /images/, /openapi.json and /scalar
	// here too until their phases land).
	notFound := http.HandlerFunc(handleNotFound)
	mux.Handle("/", withSpecValidation(spec, notFound))

	strict := gen.NewStrictHandlerWithOptions(d, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler,
		ResponseErrorHandlerFunc: responseErrorHandler,
	})
	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter: mux,
		Middlewares: []gen.MiddlewareFunc{
			func(next http.Handler) http.Handler { return withSpecValidation(spec, next) },
		},
	})

	return noStore(mux)
}
