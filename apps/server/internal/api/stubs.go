package api

import (
	"context"
	"net/http"

	"github.com/refsdal/snarvei/server/internal/api/gen"
)

// mountTestHooks is hand-routed (not part of the OpenAPI spec) and stubbed
// as a no-op until its task lands.
func (d Deps) mountTestHooks(*http.ServeMux) {}

// notImplemented is what every stub below answers: 501, until Task 10
// replaces each method one by one.
var notImplemented = fail(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Not implemented yet")

func (d Deps) ListOrganizations(context.Context, gen.ListOrganizationsRequestObject) (gen.ListOrganizationsResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) CreateOrganization(context.Context, gen.CreateOrganizationRequestObject) (gen.CreateOrganizationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) SwitchOrganization(context.Context, gen.SwitchOrganizationRequestObject) (gen.SwitchOrganizationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) ListOrganizationMembers(context.Context, gen.ListOrganizationMembersRequestObject) (gen.ListOrganizationMembersResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) ListInvitations(context.Context, gen.ListInvitationsRequestObject) (gen.ListInvitationsResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) CreateInvitation(context.Context, gen.CreateInvitationRequestObject) (gen.CreateInvitationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) CancelInvitation(context.Context, gen.CancelInvitationRequestObject) (gen.CancelInvitationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) GetInvitation(context.Context, gen.GetInvitationRequestObject) (gen.GetInvitationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) AcceptInvitation(context.Context, gen.AcceptInvitationRequestObject) (gen.AcceptInvitationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) RejectInvitation(context.Context, gen.RejectInvitationRequestObject) (gen.RejectInvitationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) RegisterWithInvitation(context.Context, gen.RegisterWithInvitationRequestObject) (gen.RegisterWithInvitationResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) ListTeams(context.Context, gen.ListTeamsRequestObject) (gen.ListTeamsResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) CreateTeam(context.Context, gen.CreateTeamRequestObject) (gen.CreateTeamResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) ListTeamMembers(context.Context, gen.ListTeamMembersRequestObject) (gen.ListTeamMembersResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) AddTeamMember(context.Context, gen.AddTeamMemberRequestObject) (gen.AddTeamMemberResponseObject, error) {
	return nil, notImplemented
}

func (d Deps) RemoveTeamMember(context.Context, gen.RemoveTeamMemberRequestObject) (gen.RemoveTeamMemberResponseObject, error) {
	return nil, notImplemented
}
