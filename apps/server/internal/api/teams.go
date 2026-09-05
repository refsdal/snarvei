package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/authz"
	"github.com/refsdal/snarvei/server/internal/db"
	dbgen "github.com/refsdal/snarvei/server/internal/db/gen"
)

func (d Deps) ListTeams(ctx context.Context, _ gen.ListTeamsRequestObject) (gen.ListTeamsResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	out := gen.ListTeams200JSONResponse{}
	if authz.IsOrgAdmin(o.Role) {
		rows, err := d.Q.ListTeams(ctx, o.OrgID)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, gen.Team{Id: r.ID, OrganizationId: r.OrganizationID, Name: r.Name, MemberCount: int(r.MemberCount), CreatedAt: ts(r.CreatedAt)})
		}
		return out, nil
	}
	rows, err := d.Q.ListTeamsForMember(ctx, dbgen.ListTeamsForMemberParams{OrganizationID: o.OrgID, UserID: o.UserID})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, gen.Team{Id: r.ID, OrganizationId: r.OrganizationID, Name: r.Name, MemberCount: int(r.MemberCount), CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

func (d Deps) CreateTeam(ctx context.Context, req gen.CreateTeamRequestObject) (gen.CreateTeamResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	name := strings.TrimSpace(req.Body.Name)
	if name == "" || len(name) > 120 {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Team name is required")
	}
	existing, err := d.Q.ListTeams(ctx, o.OrgID)
	if err != nil {
		return nil, err
	}
	for _, t := range existing {
		if strings.EqualFold(t.Name, name) {
			return nil, fail(http.StatusConflict, "TEAM_EXISTS", "A team with that name already exists")
		}
	}
	row, err := d.Q.CreateTeam(ctx, dbgen.CreateTeamParams{ID: auth.NewID(), OrganizationID: o.OrgID, Name: name})
	if err != nil {
		if db.IsUniqueViolation(err) {
			return nil, fail(http.StatusConflict, "TEAM_EXISTS", "A team with that name already exists")
		}
		return nil, err
	}
	return gen.CreateTeam201JSONResponse{Id: row.ID, OrganizationId: row.OrganizationID, Name: row.Name, MemberCount: 0, CreatedAt: ts(row.CreatedAt)}, nil
}

func (d Deps) ListTeamMembers(ctx context.Context, _ gen.ListTeamMembersRequestObject) (gen.ListTeamMembersResponseObject, error) {
	tc, _ := middleware.TeamFromContext(ctx)
	rows, err := d.Q.ListTeamMembers(ctx, tc.TeamID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListTeamMembers200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.TeamMember{UserId: r.UserID, Name: r.Name, Email: r.Email, CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

func (d Deps) AddTeamMember(ctx context.Context, req gen.AddTeamMemberRequestObject) (gen.AddTeamMemberResponseObject, error) {
	tc, _ := middleware.TeamFromContext(ctx)
	n, err := d.Q.CountOrganizationMembership(ctx, dbgen.CountOrganizationMembershipParams{OrganizationID: tc.OrgID, UserID: req.Body.UserId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "User is not a member of this organization")
	}
	if err := d.Q.AddTeamMember(ctx, dbgen.AddTeamMemberParams{TeamID: tc.TeamID, UserID: req.Body.UserId}); err != nil {
		return nil, err
	}
	return gen.AddTeamMember204Response{}, nil
}

func (d Deps) RemoveTeamMember(ctx context.Context, req gen.RemoveTeamMemberRequestObject) (gen.RemoveTeamMemberResponseObject, error) {
	tc, _ := middleware.TeamFromContext(ctx)
	removed, err := d.Q.RemoveTeamMember(ctx, dbgen.RemoveTeamMemberParams{TeamID: tc.TeamID, UserID: req.UserId})
	if err != nil {
		return nil, err
	}
	if removed == 0 {
		return nil, fail(http.StatusNotFound, "NOT_FOUND", "Not a member of this team")
	}
	return gen.RemoveTeamMember204Response{}, nil
}
