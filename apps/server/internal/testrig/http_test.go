package testrig_test

import (
	"net/http"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func TestAppRigRoundTrip(t *testing.T) {
	a := testrig.App(t)
	if resp := a.Do(http.MethodGet, "/api/config", nil, ""); resp.Code != 200 || resp.JSON["openSignup"] != true {
		t.Fatalf("config: %d %s", resp.Code, resp.Body)
	}
	orgID, ownerCookie := a.NewOrg("Acme", "acme", "owner@example.com")
	if orgID == "" || ownerCookie == "" {
		t.Fatal("NewOrg")
	}
	ownerID := a.SignUp("Second", "second@example.com")
	_ = ownerID
	if resp := a.Do(http.MethodGet, "/api/auth/me", nil, ownerCookie); resp.Code != 200 {
		t.Fatalf("limen me: %d %s", resp.Code, resp.Body)
	}
	if resp := a.Do(http.MethodGet, "/api/auth/me", nil, ""); resp.Code != 401 && resp.Code != 404 {
		t.Fatalf("anonymous limen me: %d", resp.Code)
	}
}
