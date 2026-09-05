package api

import (
	"context"
	"net/http"

	"github.com/refsdal/snarvei/server/internal/api/gen"
)

// Temporary stubs for the link operations added to the spec ahead of their
// handlers (Task 6). Deleted once real implementations land.
var notImplemented = fail(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not implemented yet")

func (d Deps) ListLinks(ctx context.Context, _ gen.ListLinksRequestObject) (gen.ListLinksResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) CreateLink(ctx context.Context, _ gen.CreateLinkRequestObject) (gen.CreateLinkResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) GetLink(ctx context.Context, _ gen.GetLinkRequestObject) (gen.GetLinkResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) UpdateLink(ctx context.Context, _ gen.UpdateLinkRequestObject) (gen.UpdateLinkResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) DeleteLink(ctx context.Context, _ gen.DeleteLinkRequestObject) (gen.DeleteLinkResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) ListLinkHistory(ctx context.Context, _ gen.ListLinkHistoryRequestObject) (gen.ListLinkHistoryResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) GetLinkAnalytics(ctx context.Context, _ gen.GetLinkAnalyticsRequestObject) (gen.GetLinkAnalyticsResponseObject, error) {
	return nil, notImplemented
}
