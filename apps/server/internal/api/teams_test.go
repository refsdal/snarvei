package api_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestTeamsAndMembership(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	ownerID := ownerIDOf(t, a, "owner@example.com")
	memberID, member := a.Join(orgID, ownerID, "member@example.com", "member")
	adminID, admin := a.Join(orgID, ownerID, "admin@example.com", "admin")
	_ = adminID
	a.SignUp("Outsider", "outsider@example.com")
	outsider := a.SignIn("outsider@example.com")

	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, member); resp.Code != 403 {
		t.Fatalf("member create team: %d", resp.Code)
	}
	created := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, admin)
	if created.Code != 201 || created.JSON["name"] != "Marketing" || created.JSON["memberCount"] != float64(0) {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	teamID := created.JSON["id"].(string)
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "marketing"}, owner); resp.Code != 409 {
		t.Fatalf("duplicate name (case-insensitive): %d %s", resp.Code, resp.Body)
	}
	second := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Sales"}, owner)

	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, owner); len(list.Array) != 2 {
		t.Fatalf("owner sees all: %s", list.Body)
	}
	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, member); len(list.Array) != 0 {
		t.Fatalf("member sees none yet: %s", list.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, outsider); resp.Code != 403 {
		t.Fatalf("outsider: %d", resp.Code)
	}

	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, member); resp.Code != 403 {
		t.Fatalf("member adds self: %d", resp.Code)
	}
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": "nobody"}, owner); resp.Code != 404 {
		t.Fatalf("unknown user: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, owner); resp.Code != 204 {
		t.Fatalf("add: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, owner); resp.Code != 204 {
		t.Fatalf("add is idempotent: %d", resp.Code)
	}
	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/teams", nil, member); len(list.Array) != 1 || list.Array[0]["id"] != teamID || list.Array[0]["memberCount"] != float64(1) {
		t.Fatalf("member sees own team: %s", list.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/teams/"+second.JSON["id"].(string)+"/members", nil, member); resp.Code != 403 {
		t.Fatalf("member reads other team: %d", resp.Code)
	}
	if resp := a.Do(http.MethodGet, "/api/teams/"+teamID+"/members", nil, member); resp.Code != 200 || len(resp.Array) != 1 || resp.Array[0]["userId"] != memberID {
		t.Fatalf("team members: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/teams/nope/members", nil, owner); resp.Code != 404 {
		t.Fatalf("unknown team: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/teams/"+teamID+"/members/"+memberID, nil, member); resp.Code != 403 {
		t.Fatalf("member removes: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/teams/"+teamID+"/members/"+memberID, nil, admin); resp.Code != 204 {
		t.Fatalf("remove: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodDelete, "/api/teams/"+teamID+"/members/"+memberID, nil, admin); resp.Code != 404 {
		t.Fatalf("remove twice: %d", resp.Code)
	}
}
