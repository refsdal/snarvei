package api_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/api"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func apiNewHandler(d api.Deps) http.Handler { return api.NewHandler(d) }

func TestMailHookListsNewestFirstAndClears(t *testing.T) {
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "a@example.com", "role": "member"}, owner)
	a.Do(http.MethodPost, "/api/organizations/"+orgID+"/invitations", map[string]string{"email": "b@example.com", "role": "member"}, owner)
	resp := a.Do(http.MethodGet, "/api/_test/mail", nil, "")
	msgs := resp.JSON["messages"].([]any)
	if resp.Code != 200 || len(msgs) != 2 || msgs[0].(map[string]any)["to"] != "b@example.com" {
		t.Fatalf("mail hook: %d %s", resp.Code, resp.Body)
	}
	if a.Do(http.MethodDelete, "/api/_test/mail", nil, "").Code != 204 {
		t.Fatal("clear")
	}
	if resp := a.Do(http.MethodGet, "/api/_test/mail", nil, ""); len(resp.JSON["messages"].([]any)) != 0 {
		t.Fatal("not cleared")
	}
}

func TestMailHookAbsentWithoutTestHooks(t *testing.T) {
	a := testrig.App(t)
	deps := a.Deps
	deps.TestHooks = false
	a.Handler = apiNewHandler(deps)
	if resp := a.Do(http.MethodGet, "/api/_test/mail", nil, ""); resp.Code != 404 {
		t.Fatalf("hook mounted without TestHooks: %d", resp.Code)
	}
}
