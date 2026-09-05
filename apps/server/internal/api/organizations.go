package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/refsdal/snarvei/server/internal/api/gen"
	"github.com/refsdal/snarvei/server/internal/api/middleware"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (d Deps) mountTestHooks(*http.ServeMux) {} // replaced in Task 11

func (d Deps) ListOrganizations(ctx context.Context, _ gen.ListOrganizationsRequestObject) (gen.ListOrganizationsResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	rows, err := d.Q.ListOrganizationsForUser(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListOrganizations200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.Organization{Id: r.ID, Name: r.Name, Slug: r.Slug, Role: gen.OrganizationRole(roleString(r.Role))})
	}
	return out, nil
}

func (d Deps) CreateOrganization(ctx context.Context, req gen.CreateOrganizationRequestObject) (gen.CreateOrganizationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	name := strings.TrimSpace(req.Body.Name)
	slug := strings.ToLower(strings.TrimSpace(req.Body.Slug))
	if name == "" || len(name) > 120 || !slugPattern.MatchString(slug) || len(slug) < 2 || len(slug) > 64 {
		return nil, fail(http.StatusBadRequest, "VALIDATION_FAILED", "Name and a slug of lowercase letters, digits and single hyphens are required")
	}
	org, err := d.Auth.CreateOrganization(ctx, s.UserID, name, slug)
	if err != nil {
		return nil, err
	}
	return gen.CreateOrganization201JSONResponse{Id: org.ID, Name: org.Name, Slug: org.Slug, Role: gen.OrganizationRoleOwner}, nil
}

func (d Deps) SwitchOrganization(ctx context.Context, _ gen.SwitchOrganizationRequestObject) (gen.SwitchOrganizationResponseObject, error) {
	s := middleware.SessionFromContext(ctx)
	o, _ := middleware.OrgFromContext(ctx)
	if err := d.Auth.SetActiveOrganization(ctx, s.Token, o.OrgID); err != nil {
		return nil, err
	}
	return gen.SwitchOrganization204Response{}, nil
}

func (d Deps) ListOrganizationMembers(ctx context.Context, _ gen.ListOrganizationMembersRequestObject) (gen.ListOrganizationMembersResponseObject, error) {
	o, _ := middleware.OrgFromContext(ctx)
	rows, err := d.Q.ListOrganizationMembers(ctx, o.OrgID)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListOrganizationMembers200JSONResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, gen.Member{Id: r.MemberID, UserId: r.UserID, Name: r.Name, Email: r.Email, Role: roleString(r.Role), CreatedAt: ts(r.CreatedAt)})
	}
	return out, nil
}

// roleString unwraps sqlc's nullable role column (a subquery result).
func roleString(v any) string {
	switch r := v.(type) {
	case string:
		return r
	case *string:
		if r != nil {
			return *r
		}
	}
	return ""
}
