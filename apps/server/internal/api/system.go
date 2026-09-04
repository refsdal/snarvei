package api

import (
	"context"

	"github.com/refsdal/snarvei/server/internal/api/gen"
)

var _ gen.StrictServerInterface = Deps{}

// Healthz is pure liveness: it reads nothing, not even d.Pool.
func (d Deps) Healthz(_ context.Context, _ gen.HealthzRequestObject) (gen.HealthzResponseObject, error) {
	return gen.Healthz200JSONResponse{
		Ok:      gen.Healthz200JSONResponseBodyOkTrue,
		Service: gen.Snarvei,
		Version: d.Version,
	}, nil
}

// Readyz runs SELECT 1 and reports 503 on failure. The failure is a normal
// response, not an error, so it never hits the 500 path.
func (d Deps) Readyz(ctx context.Context, _ gen.ReadyzRequestObject) (gen.ReadyzResponseObject, error) {
	var one int
	if err := d.Pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return gen.Readyz503JSONResponse{Ok: gen.False, Error: err.Error()}, nil
	}
	return gen.Readyz200JSONResponse{Ok: gen.Readyz200JSONResponseBodyOkTrue}, nil
}

// GetConfig is public: the landing page reads it before any session exists.
func (d Deps) GetConfig(_ context.Context, _ gen.GetConfigRequestObject) (gen.GetConfigResponseObject, error) {
	return gen.GetConfig200JSONResponse{AppName: d.AppName, OpenSignup: d.OpenSignup}, nil
}
