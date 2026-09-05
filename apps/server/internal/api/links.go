package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/links"
)

const (
	maxSlugAttempts = 10
	maxTitle        = 200
	maxDescription  = 2000
	defaultPageSize = 100
	maxPageSize     = 500
)

// pageParams normalizes page/pageSize into the values callers need: p and
// size for the response envelope, offset and limit for the query.
func pageParams(page, pageSize *int) (p int, size int, offset int32, limit int32, err error) {
	p, size = 1, defaultPageSize
	if page != nil {
		p = *page
	}
	if pageSize != nil {
		size = *pageSize
	}
	if p < 1 || size < 1 || size > maxPageSize {
		return 0, 0, 0, 0, fail(http.StatusBadRequest, "VALIDATION_FAILED", "page must be >= 1 and pageSize 1..500")
	}
	off64 := int64(p-1) * int64(size)
	if off64 > math.MaxInt32 {
		return 0, 0, 0, 0, fail(http.StatusBadRequest, "VALIDATION_FAILED", "page too large")
	}
	return p, size, int32(off64), int32(size), nil
}

func toLink(r dbgen.GetLinkRow) gen.Link {
	return gen.Link{
		Id: r.ID, OrganizationId: r.OrganizationID, TeamId: r.TeamID, TeamName: r.TeamName, Slug: r.Slug, TargetUrl: r.TargetUrl,
		RedirectStatus: gen.LinkRedirectStatus(r.RedirectStatus), IsActive: r.IsActive, Title: r.Title, Description: r.Description,
		CreatedBy: r.CreatedBy, UpdatedBy: r.UpdatedBy, CreatedAt: ts(r.CreatedAt), UpdatedAt: ts(r.UpdatedAt),
	}
}

// linkForCaller loads a link and checks the caller may act on its team.
// Non-members of the organization get NOT_FOUND (existence is not revealed);
// org members outside the team get FORBIDDEN.
func (d Deps) linkForCaller(ctx context.Context, linkID string) (dbgen.GetLinkRow, middleware.TeamCtx, error) {
	s := middleware.SessionFromContext(ctx)
	row, err := d.Q.GetLink(ctx, linkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, middleware.TeamCtx{}, fail(http.StatusNotFound, "NOT_FOUND", "Link not found")
	}
	if err != nil {
		return row, middleware.TeamCtx{}, err
	}
	tc, err := middleware.ResolveTeamAccess(ctx, d.mwDeps(), s.UserID, row.TeamID)
	if errors.Is(err, middleware.ErrTeamForbidden) {
		if tc.Role == "" {
			return row, tc, fail(http.StatusNotFound, "NOT_FOUND", "Link not found")
		}
		return row, tc, fail(http.StatusForbidden, "FORBIDDEN", "Team access denied")
	}
	if err != nil {
		return row, tc, err
	}
	return row, tc, nil
}

// optionalText trims and turns blank into nil; nil stays nil (the caller
// keeps the current value in that case).
func optionalText(v *string, max int) (*string, error) {
	if v == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*v)
	if len(t) > max {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Text is too long")
	}
	if t == "" {
		return nil, nil
	}
	return &t, nil
}

// inTx runs fn in one transaction with a transaction-bound Queries.
func (d Deps) inTx(ctx context.Context, fn func(q *dbgen.Queries) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(dbgen.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d Deps) CreateLink(ctx context.Context, req gen.CreateLinkRequestObject) (gen.CreateLinkResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	tc, err := middleware.ResolveTeamAccess(ctx, d.mwDeps(), s.UserID, req.Body.TeamId)
	if errors.Is(err, middleware.ErrTeamNotFound) {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "Team not found")
	}
	if errors.Is(err, middleware.ErrTeamForbidden) {
		return nil, fail(http.StatusForbidden, "FORBIDDEN", "Team access denied")
	}
	if err != nil {
		return nil, err
	}
	target, err := links.ValidateTargetURL(req.Body.TargetUrl)
	if err != nil {
		return nil, err
	}
	status := 302
	if req.Body.RedirectStatus != nil {
		status = int(*req.Body.RedirectStatus)
	}
	if !links.ValidRedirectStatus(status) {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "redirectStatus must be 301, 302 or 307")
	}
	title, err := optionalText(req.Body.Title, maxTitle)
	if err != nil {
		return nil, err
	}
	description, err := optionalText(req.Body.Description, maxDescription)
	if err != nil {
		return nil, err
	}
	custom := ""
	if req.Body.Slug != nil {
		if custom, err = links.NormalizeCustomSlug(*req.Body.Slug); err != nil {
			return nil, err
		}
	}

	id := auth.NewID()
	for attempt := 0; ; attempt++ {
		slug := custom
		if slug == "" {
			slug = links.GenerateSlug()
		}
		err := d.inTx(ctx, func(q *dbgen.Queries) error {
			if err := q.CreateLink(ctx, dbgen.CreateLinkParams{ID: id, OrganizationID: tc.OrgID, TeamID: tc.TeamID, Slug: slug, TargetUrl: target, RedirectStatus: int16(status), Title: title, Description: description, CreatedBy: &s.UserID}); err != nil {
				return err
			}
			return q.InsertLinkHistory(ctx, dbgen.InsertLinkHistoryParams{ID: auth.NewID(), LinkID: id, OldTargetUrl: nil, NewTargetUrl: target, ChangedBy: &s.UserID})
		})
		if err == nil {
			break
		}
		if db.IsUniqueViolation(err) {
			if custom != "" {
				return nil, fail(http.StatusConflict, "SLUG_TAKEN", "That slug is already taken")
			}
			if attempt < maxSlugAttempts-1 {
				continue
			}
		}
		return nil, err
	}
	row, err := d.Q.GetLink(ctx, id)
	if err != nil {
		return nil, err
	}
	return gen.CreateLink201JSONResponse(toLink(row)), nil
}

func (d Deps) GetLink(ctx context.Context, req gen.GetLinkRequestObject) (gen.GetLinkResponseObject, error) {
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	return gen.GetLink200JSONResponse(toLink(row)), nil
}

func (d Deps) UpdateLink(ctx context.Context, req gen.UpdateLinkRequestObject) (gen.UpdateLinkResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	target := row.TargetUrl
	if req.Body.TargetUrl != nil {
		if target, err = links.ValidateTargetURL(*req.Body.TargetUrl); err != nil {
			return nil, err
		}
	}
	status := int(row.RedirectStatus)
	if req.Body.RedirectStatus != nil {
		status = int(*req.Body.RedirectStatus)
		if !links.ValidRedirectStatus(status) {
			return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "redirectStatus must be 301, 302 or 307")
		}
	}
	active := row.IsActive
	if req.Body.IsActive != nil {
		active = *req.Body.IsActive
	}
	title, description := row.Title, row.Description
	if req.Body.Title != nil { // present (string or JSON null indistinguishable): blank/null clears
		if title, err = optionalText(req.Body.Title, maxTitle); err != nil {
			return nil, err
		}
	}
	if req.Body.Description != nil {
		if description, err = optionalText(req.Body.Description, maxDescription); err != nil {
			return nil, err
		}
	}
	err = d.inTx(ctx, func(q *dbgen.Queries) error {
		if err := q.UpdateLink(ctx, dbgen.UpdateLinkParams{ID: row.ID, TargetUrl: target, RedirectStatus: int16(status), IsActive: active, Title: title, Description: description, UpdatedBy: &s.UserID}); err != nil {
			return err
		}
		if target != row.TargetUrl {
			old := row.TargetUrl
			return q.InsertLinkHistory(ctx, dbgen.InsertLinkHistoryParams{ID: auth.NewID(), LinkID: row.ID, OldTargetUrl: &old, NewTargetUrl: target, ChangedBy: &s.UserID})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	updated, err := d.Q.GetLink(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	return gen.UpdateLink200JSONResponse(toLink(updated)), nil
}

func (d Deps) DeleteLink(ctx context.Context, req gen.DeleteLinkRequestObject) (gen.DeleteLinkResponseObject, error) {
	row, _, err := d.linkForCaller(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	if _, err := d.Q.DeleteLink(ctx, row.ID); err != nil {
		return nil, err
	}
	return gen.DeleteLink204Response{}, nil
}

func (d Deps) ListLinks(ctx context.Context, req gen.ListLinksRequestObject) (gen.ListLinksResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	page, size, offset, limit, err := pageParams(req.Params.Page, req.Params.PageSize)
	if err != nil {
		return nil, err
	}
	orgID := req.Params.OrganizationId
	roles, err := d.Q.GetMemberRoles(ctx, dbgen.GetMemberRolesParams{OrganizationID: orgID, UserID: s.UserID})
	if err != nil {
		return nil, err
	}
	role := authz.Highest(roles)
	if role == "" {
		return nil, fail(http.StatusForbidden, "FORBIDDEN", "Organization access denied")
	}
	var teamID *string
	var teamIDs []string
	if req.Params.TeamId != nil && *req.Params.TeamId != "" {
		tc, err := middleware.ResolveTeamAccess(ctx, d.mwDeps(), s.UserID, *req.Params.TeamId)
		if errors.Is(err, middleware.ErrTeamNotFound) {
			return nil, fail(http.StatusNotFound, "NOT_FOUND", "Team not found")
		}
		if err != nil && !errors.Is(err, middleware.ErrTeamForbidden) {
			return nil, err
		}
		// tc is populated whether err is nil or ErrTeamForbidden; a team that
		// belongs to a different organization is reported as not found
		// regardless of access, before a forbidden verdict from its own org.
		if tc.OrgID != orgID {
			return nil, fail(http.StatusNotFound, "NOT_FOUND", "Team not found")
		}
		if errors.Is(err, middleware.ErrTeamForbidden) {
			return nil, fail(http.StatusForbidden, "FORBIDDEN", "Team access denied")
		}
		teamID = req.Params.TeamId
	} else if !authz.IsOrgAdmin(role) {
		ids, err := d.Q.ListAccessibleTeamIDs(ctx, dbgen.ListAccessibleTeamIDsParams{OrganizationID: orgID, UserID: s.UserID})
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return gen.ListLinks200JSONResponse{Items: []gen.Link{}, Page: page, PageSize: size, Total: 0}, nil
		}
		teamIDs = ids
	}
	rows, err := d.Q.ListLinks(ctx, dbgen.ListLinksParams{OrganizationID: orgID, TeamID: teamID, TeamIds: teamIDs, PageSize: limit, PageOffset: offset})
	if err != nil {
		return nil, err
	}
	total, err := d.Q.CountLinks(ctx, dbgen.CountLinksParams{OrganizationID: orgID, TeamID: teamID, TeamIds: teamIDs})
	if err != nil {
		return nil, err
	}
	items := make([]gen.Link, 0, len(rows))
	for _, r := range rows {
		items = append(items, toLink(dbgen.GetLinkRow(r)))
	}
	return gen.ListLinks200JSONResponse{Items: items, Page: page, PageSize: size, Total: int(total)}, nil
}

func toHistoryItem(r dbgen.LinkTargetHistory) gen.HistoryItem {
	return gen.HistoryItem{
		Id: r.ID, LinkId: r.LinkID, OldTargetUrl: r.OldTargetUrl, NewTargetUrl: r.NewTargetUrl,
		ChangedBy: r.ChangedBy, ChangedAt: ts(r.ChangedAt),
	}
}

func (d Deps) ListLinkHistory(ctx context.Context, req gen.ListLinkHistoryRequestObject) (gen.ListLinkHistoryResponseObject, error) {
	if _, _, err := d.linkForCaller(ctx, req.LinkId); err != nil {
		return nil, err
	}
	page, size, offset, limit, err := pageParams(req.Params.Page, req.Params.PageSize)
	if err != nil {
		return nil, err
	}
	rows, err := d.Q.ListLinkHistory(ctx, dbgen.ListLinkHistoryParams{LinkID: req.LinkId, Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	total, err := d.Q.CountLinkHistory(ctx, req.LinkId)
	if err != nil {
		return nil, err
	}
	items := make([]gen.HistoryItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, toHistoryItem(r))
	}
	return gen.ListLinkHistory200JSONResponse{Items: items, Page: page, PageSize: size, Total: int(total)}, nil
}
