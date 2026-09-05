package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

func optTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func firstRole(raw string) string {
	roles := auth.RolesFromJSON(raw)
	if len(roles) == 0 {
		return ""
	}
	return roles[0]
}

func (d Deps) ListInvitations(ctx context.Context, _ gen.ListInvitationsRequestObject) (gen.ListInvitationsResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	rows, err := d.Q.ListPendingInvitations(ctx, o.OrgID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListInvitations200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.Invitation{Id: r.ID, Email: r.Email, Role: firstRole(r.Roles), Status: r.Status, ExpiresAt: optTime(r.ExpiresAt), TeamId: r.TeamID, TeamName: r.TeamName, CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

func (d Deps) CreateInvitation(ctx context.Context, req gen.CreateInvitationRequestObject) (gen.CreateInvitationResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	role := string(req.Body.Role)
	if !authz.IsValidInviteRole(role) {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Role must be admin or member")
	}
	var team *dbgen.Teams
	if req.Body.TeamId != nil && *req.Body.TeamId != "" {
		t, err := d.Q.GetTeam(ctx, *req.Body.TeamId)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.OrganizationID != o.OrgID) {
			return nil, fail(http.StatusNotFound, "NOT_FOUND", "Team not found")
		}
		if err != nil {
			return nil, err
		}
		team = &t
	}
	inv, err := d.Auth.CreateInvitation(ctx, o.UserID, o.OrgID, strings.TrimSpace(string(req.Body.Email)), role)
	if err != nil {
		return nil, err
	}
	if team != nil {
		if err := d.Q.SetInvitationTeam(ctx, dbgen.SetInvitationTeamParams{InvitationID: inv.ID, TeamID: team.ID}); err != nil {
			return nil, err
		}
	}
	row, err := d.Q.GetInvitation(ctx, inv.ID)
	if err != nil {
		return nil, err
	}
	return gen.CreateInvitation201JSONResponse{Id: row.ID, Email: row.Email, Role: firstRole(row.Roles), Status: row.Status, ExpiresAt: optTime(row.ExpiresAt), TeamId: row.TeamID, TeamName: row.TeamName, CreatedAt: ts(row.CreatedAt)}, nil
}

func (d Deps) CancelInvitation(ctx context.Context, req gen.CancelInvitationRequestObject) (gen.CancelInvitationResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	row, err := d.Q.GetInvitation(ctx, req.InvitationId)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (row.OrganizationID != o.OrgID || row.Status != "pending")) {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "Invitation not found")
	}
	if err != nil {
		return nil, err
	}
	if err := d.Auth.CancelInvitation(ctx, o.UserID, o.OrgID, req.InvitationId); err != nil {
		return nil, err
	}
	return gen.CancelInvitation204Response{}, nil
}

func (d Deps) GetInvitation(ctx context.Context, req gen.GetInvitationRequestObject) (gen.GetInvitationResponseObject, error) {
	row, err := d.Q.GetInvitation(ctx, req.InvitationId)
	if err != nil {
		return nil, err
	}
	n, err := d.Q.CountUsersByEmail(ctx, row.Email)
	if err != nil {
		return nil, err
	}
	var inviter *string
	if row.InviterName != "" {
		inviter = &row.InviterName
	}
	return gen.GetInvitation200JSONResponse{Id: row.ID, OrganizationName: row.OrganizationName, InviterName: inviter, Role: firstRole(row.Roles), Status: row.Status, TeamName: row.TeamName, ExpiresAt: optTime(row.ExpiresAt), HasAccount: n > 0}, nil
}

// joinTeamIfInvited adds the team membership an invitation carried.
func (d Deps) joinTeamIfInvited(ctx context.Context, invitationID, userID string) error {
	row, err := d.Q.GetInvitation(ctx, invitationID)
	if err != nil {
		return err
	}
	if row.TeamID != nil {
		return d.Q.AddTeamMember(ctx, dbgen.AddTeamMemberParams{TeamID: *row.TeamID, UserID: userID})
	}
	return nil
}

func (d Deps) AcceptInvitation(ctx context.Context, req gen.AcceptInvitationRequestObject) (gen.AcceptInvitationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	inv, err := d.Auth.AcceptInvitation(ctx, s.UserID, req.InvitationId)
	if err != nil {
		return nil, err
	}
	if err := d.joinTeamIfInvited(ctx, inv.ID, s.UserID); err != nil {
		return nil, err
	}
	if err := d.Auth.SetActiveOrganization(ctx, s.Token, inv.OrganizationID); err != nil {
		return nil, err
	}
	org, err := d.Q.GetOrganization(ctx, inv.OrganizationID)
	if err != nil {
		return nil, err
	}
	return gen.AcceptInvitation200JSONResponse{Id: org.ID, Name: org.Name, Slug: org.Slug, Role: gen.OrganizationRole(inv.Role)}, nil
}

func (d Deps) RejectInvitation(ctx context.Context, req gen.RejectInvitationRequestObject) (gen.RejectInvitationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	if err := d.Auth.RejectInvitation(ctx, s.UserID, req.InvitationId); err != nil {
		return nil, err
	}
	return gen.RejectInvitation204Response{}, nil
}

func (d Deps) RegisterWithInvitation(ctx context.Context, req gen.RegisterWithInvitationRequestObject) (gen.RegisterWithInvitationResponseObject, error) {
	row, err := d.Q.GetInvitation(ctx, req.InvitationId)
	if err != nil {
		return nil, err
	}
	if row.Status != "pending" || (row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now())) {
		return nil, fail(http.StatusGone, "INVITATION_INVALID", "This invitation is no longer valid")
	}
	if n, err := d.Q.CountUsersByEmail(ctx, row.Email); err != nil {
		return nil, err
	} else if n > 0 {
		return nil, fail(http.StatusConflict, "EMAIL_TAKEN", "An account already exists for this email; sign in to accept")
	}
	w, r, ok := middleware.HTTPFromContext(ctx)
	if !ok {
		return nil, errors.New("api: RegisterWithInvitation needs CaptureHTTP")
	}
	userID, err := d.Auth.CreateUser(ctx, req.Body.Name, row.Email, req.Body.Password)
	if err != nil {
		return nil, err
	}
	inv, err := d.Auth.AcceptInvitation(ctx, userID, req.InvitationId)
	if err != nil {
		return nil, err
	}
	if err := d.joinTeamIfInvited(ctx, inv.ID, userID); err != nil {
		return nil, err
	}
	if err := d.Auth.StartSession(ctx, w, r, userID); err != nil {
		return nil, err
	}
	// The cookie is on w; resolve the fresh session from the Set-Cookie value
	// to activate the organization and build the Me body.
	token := ""
	for _, c := range (&http.Response{Header: w.Header()}).Cookies() {
		if c.Name == auth.SessionCookieName {
			token = c.Value
		}
	}
	if token == "" {
		return nil, errors.New("api: StartSession set no cookie")
	}
	if err := d.Auth.SetActiveOrganization(ctx, token, inv.OrganizationID); err != nil {
		return nil, err
	}
	probe := r.Clone(ctx)
	probe.Header = http.Header{"Cookie": {auth.SessionCookieName + "=" + token}}
	s, err := d.Auth.SessionFromRequest(nil, probe)
	if err != nil || s == nil {
		return nil, errors.New("api: fresh session did not resolve")
	}
	me, err := d.me(ctx, s)
	if err != nil {
		return nil, err
	}
	return gen.RegisterWithInvitation201JSONResponse(me), nil
}
