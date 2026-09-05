package api

import (
	"context"
	"net/http"

	"github.com/refsdal/snarvei/server/internal/api/gen"
)

// Temporary stub for the link analytics operation added to the spec ahead of
// its handler (Task 6). Deleted once the real implementation lands.
var notImplemented = fail(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not implemented yet")

func (d Deps) GetLinkAnalytics(ctx context.Context, _ gen.GetLinkAnalyticsRequestObject) (gen.GetLinkAnalyticsResponseObject, error) {
	return nil, notImplemented
}
