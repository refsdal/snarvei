package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestInvitationFlowWithTeam(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	team := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, owner)
	if team.Code != 201 {
		t.Fatalf("team: %d %s", team.Code, team.Body)
	}
	teamID := team.JSON["id"].(string)

	resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "new@example.com", "role": "member", "teamId": teamID}, owner)
	if resp.Code != 201 || resp.JSON["teamId"] != teamID || resp.JSON["role"] != "member" || resp.JSON["status"] != "pending" {
		t.Fatalf("invite: %d %s", resp.Code, resp.Body)
	}
	invID := resp.JSON["id"].(string)
	msg, ok := a.Mail.Last("new@example.com")
	if !ok || !strings.Contains(msg.Text, "/app/invitations/"+invID) {
		t.Fatalf("mail: %+v", msg)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "new@example.com", "role": "member"}, owner); resp.Code != 409 || resp.JSON["code"] != "INVITATION_EXISTS" {
		t.Fatalf("duplicate: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "x@example.com", "role": "member", "teamId": "nope"}, owner); resp.Code != 404 {
		t.Fatalf("unknown team: %d %s", resp.Code, resp.Body)
	}
	list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/invitations", nil, owner)
	if list.Code != 200 || len(list.Array) != 1 || list.Array[0]["teamName"] != "Marketing" {
		t.Fatalf("list: %d %s", list.Code, list.Body)
	}

	// Public view: no email, hasAccount false.
	pub := a.Do(http.MethodGet, "/api/invitations/"+invID, nil, "")
	if pub.Code != 200 || pub.JSON["organizationName"] != "Acme" || pub.JSON["teamName"] != "Marketing" || pub.JSON["hasAccount"] != false || strings.Contains(string(pub.Body), "new@example.com") {
		t.Fatalf("public: %d %s", pub.Code, pub.Body)
	}
	if a.Do(http.MethodGet, "/api/invitations/nope", nil, "").Code != 404 {
		t.Fatal("unknown invitation")
	}

	// Register through the invitation: account created, accepted, signed in, team joined.
	reg := a.Do(http.MethodPost, "/api/invitations/"+invID+"/register", map[string]string{"name": "New Person", "password": testrig.Password}, "")
	if reg.Code != 201 || reg.JSON["user"].(map[string]any)["email"] != "new@example.com" {
		t.Fatalf("register: %d %s", reg.Code, reg.Body)
	}
	var newCookie string
	for _, c := range (&http.Response{Header: reg.Header}).Cookies() {
		if c.Name == "snarvei_session" {
			newCookie = c.Name + "=" + c.Value
		}
	}
	if newCookie == "" {
		t.Fatal("register must set the session cookie")
	}
	if reg.JSON["session"].(map[string]any)["activeOrganizationId"] != orgID {
		t.Fatalf("register must activate the organization: %s", reg.Body)
	}
	members := a.Do(http.MethodGet, "/api/teams/"+teamID+"/members", nil, newCookie)
	if members.Code != 200 || len(members.Array) != 1 || members.Array[0]["email"] != "new@example.com" {
		t.Fatalf("team members after register: %d %s", members.Code, members.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/register", map[string]string{"name": "Again", "password": testrig.Password}, ""); resp.Code != 410 {
		t.Fatalf("register twice: %d %s", resp.Code, resp.Body)
	}
	if pub := a.Do(http.MethodGet, "/api/invitations/"+invID, nil, ""); pub.JSON["status"] != "accepted" || pub.JSON["hasAccount"] != true {
		t.Fatalf("public after accept: %s", pub.Body)
	}
}

func TestInvitationAcceptRejectCancel(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	a.SignUp("Existing", "existing@example.com")
	existing := a.SignIn("existing@example.com")
	a.SignUp("Other", "other@example.com")
	other := a.SignIn("other@example.com")

	inv := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "existing@example.com", "role": "admin"}, owner)
	invID := inv.JSON["id"].(string)
	if pub := a.Do(http.MethodGet, "/api/invitations/"+invID, nil, ""); pub.JSON["hasAccount"] != true {
		t.Fatalf("hasAccount: %s", pub.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/register", map[string]string{"name": "X", "password": testrig.Password}, ""); resp.Code != 409 || resp.JSON["code"] != "EMAIL_TAKEN" {
		t.Fatalf("register with existing account: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, ""); resp.Code != 401 {
		t.Fatalf("anonymous accept: %d", resp.Code)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, other); resp.Code != 403 || resp.JSON["code"] != "INVITATION_EMAIL_MISMATCH" {
		t.Fatalf("wrong user accept: %d %s", resp.Code, resp.Body)
	}
	resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, existing)
	if resp.Code != 200 || resp.JSON["id"] != orgID || resp.JSON["role"] != "admin" {
		t.Fatalf("accept: %d %s", resp.Code, resp.Body)
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, existing); me.JSON["session"].(map[string]any)["activeOrganizationId"] != orgID {
		t.Fatal("accept must activate the organization")
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+invID+"/accept", nil, existing); resp.Code != 410 {
		t.Fatalf("accept twice: %d", resp.Code)
	}

	inv2 := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "other@example.com", "role": "member"}, existing) // admins may invite
	if inv2.Code != 201 {
		t.Fatalf("admin invite: %d %s", inv2.Code, inv2.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/invitations/"+inv2.JSON["id"].(string)+"/reject", nil, other); resp.Code != 204 {
		t.Fatalf("reject: %d %s", resp.Code, resp.Body)
	}
	if list := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/invitations", nil, owner); len(list.Array) != 0 {
		t.Fatalf("rejected must leave the pending list: %s", list.Body)
	}

	inv3 := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "third@example.com", "role": "member"}, owner)
	inv3ID := inv3.JSON["id"].(string)
	_, memberCookie := a.Join(orgID, ownerIDOf(t, a, "owner@example.com"), "plain@example.com", "member")
	if resp := a.Do(http.MethodDelete, "/api/organizations/"+orgID+"/invitations/"+inv3ID, nil, memberCookie); resp.Code != 403 {
		t.Fatalf("member cancel: %d", resp.Code)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "z@example.com", "role": "member"}, memberCookie); resp.Code != 403 {
		t.Fatalf("member invite: %d", resp.Code)
	}
	if resp := a.Do(http.MethodDelete, "/api/organizations/"+orgID+"/invitations/"+inv3ID, nil, owner); resp.Code != 204 {
		t.Fatalf("cancel: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodDelete, "/api/organizations/"+orgID+"/invitations/"+inv3ID, nil, owner); resp.Code != 404 {
		t.Fatalf("cancel twice: %d", resp.Code)
	}
}

func ownerIDOf(t *testing.T, a *testrig.AppRig, email string) string {
	t.Helper()
	var id string
	if err := a.Rig.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRegisterIsRateLimited(t *testing.T) {
	a := testrig.App(t)
	for i := 0; i < 31; i++ {
		resp := a.Do(http.MethodPost, "/api/invitations/nope/register", map[string]string{"name": "X", "password": testrig.Password}, "")
		if i < 30 && resp.Code != 404 {
			t.Fatalf("attempt %d: %d %s", i, resp.Code, resp.Body)
		}
		if i == 30 && (resp.Code != 429 || resp.Header.Get("Retry-After") == "") {
			t.Fatalf("attempt 31: %d", resp.Code)
		}
	}
}
