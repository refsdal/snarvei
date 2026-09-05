package api_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestOrganizationsCreateListSwitch(t *testing.T) {
	a := testrig.App(t)
	a.SignUp("Kari", "kari@example.com")
	cookie := a.SignIn("kari@example.com")

	if resp := a.Do(http.MethodGet, "/api/organizations", nil, cookie); resp.Code != 200 || len(resp.Array) != 0 {
		t.Fatalf("empty list: %d %s", resp.Code, resp.Body)
	}
	resp := a.Do(http.MethodPost, "/api/organizations", map[string]string{"name": "Acme", "slug": "acme"}, cookie)
	if resp.Code != 201 || resp.JSON["slug"] != "acme" || resp.JSON["role"] != "owner" {
		t.Fatalf("create: %d %s", resp.Code, resp.Body)
	}
	orgID := resp.JSON["id"].(string)
	if resp := a.Do(http.MethodPost, "/api/organizations", map[string]string{"name": "Acme 2", "slug": "acme"}, cookie); resp.Code != 409 || resp.JSON["code"] != "SLUG_TAKEN" {
		t.Fatalf("slug taken: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodPost, "/api/organizations", map[string]string{"name": "Bad", "slug": "Not Valid"}, cookie); resp.Code != 400 {
		t.Fatalf("bad slug: %d %s", resp.Code, resp.Body)
	}
	list := a.Do(http.MethodGet, "/api/organizations", nil, cookie)
	if len(list.Array) != 1 || list.Array[0]["id"] != orgID || list.Array[0]["role"] != "owner" {
		t.Fatalf("list: %s", list.Body)
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, cookie); me.JSON["session"].(map[string]any)["activeOrganizationId"] != nil {
		t.Fatal("creating must not switch by itself")
	}
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/switch", nil, cookie); resp.Code != 204 {
		t.Fatalf("switch: %d %s", resp.Code, resp.Body)
	}
	if me := a.Do(http.MethodGet, "/api/me", nil, cookie); me.JSON["session"].(map[string]any)["activeOrganizationId"] != orgID {
		t.Fatalf("active org: %s", me.Body)
	}
	a.SignUp("Stranger", "stranger@example.com")
	stranger := a.SignIn("stranger@example.com")
	if resp := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/switch", nil, stranger); resp.Code != 403 {
		t.Fatalf("stranger switch: %d", resp.Code)
	}
	members := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/members", nil, cookie)
	if members.Code != 200 || len(members.Array) != 1 || members.Array[0]["role"] != "owner" || members.Array[0]["email"] != "kari@example.com" {
		t.Fatalf("members: %d %s", members.Code, members.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/organizations/"+orgID+"/members", nil, stranger); resp.Code != 403 {
		t.Fatalf("stranger members: %d", resp.Code)
	}
}
