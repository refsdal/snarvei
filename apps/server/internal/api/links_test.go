package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

type linkFixture struct {
	a                      *testrig.AppRig
	orgID, teamID, other   string
	owner, member, outside string // cookies: owner; member of teamID; org member outside teamID
	stranger               string // cookie of a user in another org
}

func newLinkFixture(t *testing.T) *linkFixture {
	t.Helper()
	a := testrig.App(t)
	orgID, owner := a.NewOrg("Acme", "acme", "owner@example.com")
	ownerID := ownerIDOf(t, a, "owner@example.com")
	memberID, member := a.Join(orgID, ownerID, "member@example.com", "member")
	_, outside := a.Join(orgID, ownerID, "outside@example.com", "member")
	_, stranger := a.NewOrg("Other", "other", "stranger@example.com")
	team := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Marketing"}, owner)
	other := a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Sales"}, owner)
	teamID := team.JSON["id"].(string)
	if resp := a.Do(http.MethodPost, "/api/teams/"+teamID+"/members", map[string]string{"userId": memberID}, owner); resp.Code != 204 {
		t.Fatalf("add member: %d %s", resp.Code, resp.Body)
	}
	return &linkFixture{a: a, orgID: orgID, teamID: teamID, other: other.JSON["id"].(string), owner: owner, member: member, outside: outside, stranger: stranger}
}

func (f *linkFixture) create(t *testing.T, cookie string, body map[string]any) testrig.Response {
	t.Helper()
	if _, ok := body["teamId"]; !ok {
		body["teamId"] = f.teamID
	}
	return f.a.Do(http.MethodPost, "/api/links", body, cookie)
}

func TestCreateLinkRules(t *testing.T) {
	f := newLinkFixture(t)
	resp := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/launch"})
	if resp.Code != 201 {
		t.Fatalf("create: %d %s", resp.Code, resp.Body)
	}
	slug := resp.JSON["slug"].(string)
	if len(slug) != 8 || strings.ContainsAny(slug, "0OIl1") || resp.JSON["redirectStatus"] != float64(302) || resp.JSON["isActive"] != true || resp.JSON["teamName"] != "Marketing" || resp.JSON["organizationId"] != f.orgID {
		t.Fatalf("created link: %s", resp.Body)
	}
	if resp.JSON["title"] != nil || resp.JSON["createdBy"] == nil {
		t.Fatalf("defaults: %s", resp.Body)
	}
	// initial history row
	hist := f.a.Do(http.MethodGet, "/api/links/"+resp.JSON["id"].(string)+"/history", nil, f.owner)
	if hist.Code != 200 || hist.JSON["total"] != float64(1) {
		t.Fatalf("initial history: %d %s", hist.Code, hist.Body)
	}

	custom := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/x", "slug": "  Summer-2026 ", "title": "  Campaign  ", "description": "   "})
	if custom.Code != 201 || custom.JSON["slug"] != "summer-2026" || custom.JSON["title"] != "Campaign" || custom.JSON["description"] != nil {
		t.Fatalf("custom slug: %d %s", custom.Code, custom.Body)
	}
	if dup := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/y", "slug": "summer-2026"}); dup.Code != 409 || dup.JSON["code"] != "SLUG_TAKEN" {
		t.Fatalf("dup slug: %d %s", dup.Code, dup.Body)
	}
	// taken across organizations too
	if dup := f.a.Do(http.MethodPost, "/api/links", map[string]any{"teamId": f.strangerTeam(t), "targetUrl": "https://example.com/z", "slug": "summer-2026"}, f.stranger); dup.Code != 409 {
		t.Fatalf("cross-org slug: %d %s", dup.Code, dup.Body)
	}
	for _, bad := range []map[string]any{
		{"targetUrl": "javascript:alert(1)"}, {"targetUrl": "https://user:pw@example.com/"}, {"targetUrl": "example.com"},
		{"targetUrl": "https://example.com", "slug": "Hello World!"}, {"targetUrl": "https://example.com", "slug": "ab"},
		{"targetUrl": "https://example.com", "redirectStatus": 308},
	} {
		if resp := f.create(t, f.owner, bad); resp.Code != 400 || resp.JSON["code"] != "VALIDATION_FAILED" {
			t.Errorf("%v: %d %s", bad, resp.Code, resp.Body)
		}
	}
	if resp := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com", "teamId": "nope"}); resp.Code != 404 {
		t.Fatalf("unknown team: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, f.outside, map[string]any{"targetUrl": "https://example.com"}); resp.Code != 403 {
		t.Fatalf("org member outside the team: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, f.stranger, map[string]any{"targetUrl": "https://example.com"}); resp.Code != 403 {
		t.Fatalf("stranger: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, f.member, map[string]any{"targetUrl": "https://example.com"}); resp.Code != 201 {
		t.Fatalf("team member: %d %s", resp.Code, resp.Body)
	}
	if resp := f.create(t, "", map[string]any{"targetUrl": "https://example.com"}); resp.Code != 401 {
		t.Fatalf("anonymous: %d", resp.Code)
	}
}

// strangerTeam creates a team in the stranger's organization.
func (f *linkFixture) strangerTeam(t *testing.T) string {
	t.Helper()
	orgs := f.a.Do(http.MethodGet, "/api/organizations", nil, f.stranger)
	orgID := orgs.Array[0]["id"].(string)
	team := f.a.Do(http.MethodPost, "/api/organizations/"+orgID+"/teams", map[string]string{"name": "Theirs"}, f.stranger)
	return team.JSON["id"].(string)
}

func TestGetUpdateDeleteLink(t *testing.T) {
	f := newLinkFixture(t)
	created := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/v1"})
	id := created.JSON["id"].(string)

	for cookie, want := range map[string]int{f.owner: 200, f.member: 200, f.outside: 403, f.stranger: 404, "": 401} {
		if resp := f.a.Do(http.MethodGet, "/api/links/"+id, nil, cookie); resp.Code != want {
			t.Errorf("get as %q: %d want %d", cookie[:min(12, len(cookie))], resp.Code, want)
		}
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/nope", nil, f.owner); resp.Code != 404 {
		t.Fatalf("unknown: %d", resp.Code)
	}

	// title-only edit: no history row
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"title": "Renamed"}, f.member); resp.Code != 200 || resp.JSON["title"] != "Renamed" {
		t.Fatalf("patch title: %d %s", resp.Code, resp.Body)
	}
	if hist := f.a.Do(http.MethodGet, "/api/links/"+id+"/history", nil, f.owner); hist.JSON["total"] != float64(1) {
		t.Fatalf("title edit added history: %s", hist.Body)
	}
	// retarget: history row with old and new
	resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"targetUrl": "https://example.com/v2", "redirectStatus": 307}, f.owner)
	if resp.Code != 200 || resp.JSON["targetUrl"] != "https://example.com/v2" || resp.JSON["redirectStatus"] != float64(307) {
		t.Fatalf("retarget: %d %s", resp.Code, resp.Body)
	}
	hist := f.a.Do(http.MethodGet, "/api/links/"+id+"/history", nil, f.owner)
	items := hist.JSON["items"].([]any)
	if hist.JSON["total"] != float64(2) || items[0].(map[string]any)["oldTargetUrl"] != "https://example.com/v1" || items[0].(map[string]any)["newTargetUrl"] != "https://example.com/v2" {
		t.Fatalf("history: %s", hist.Body)
	}
	// blank clears, null clears, absent keeps
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"title": "", "description": "  "}, f.owner); resp.JSON["title"] != nil || resp.JSON["description"] != nil {
		t.Fatalf("clear: %s", resp.Body)
	}
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"targetUrl": "javascript:alert(1)"}, f.owner); resp.Code != 400 {
		t.Fatalf("bad retarget: %d", resp.Code)
	}
	if got := f.a.Do(http.MethodGet, "/api/links/"+id, nil, f.owner); got.JSON["targetUrl"] != "https://example.com/v2" {
		t.Fatal("bad retarget must keep the old target")
	}
	// slug in the body is rejected by the spec (additionalProperties are allowed by default) — it must be ignored
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"slug": "changed"}, f.owner); resp.Code != 200 || resp.JSON["slug"] != created.JSON["slug"] {
		t.Fatalf("slug must not change: %d %s", resp.Code, resp.Body)
	}
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"isActive": false}, f.outside); resp.Code != 403 {
		t.Fatalf("outsider patch: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"isActive": false}, f.stranger); resp.Code != 404 {
		t.Fatalf("stranger patch: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodDelete, "/api/links/"+id, nil, f.outside); resp.Code != 403 {
		t.Fatalf("outsider delete: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodDelete, "/api/links/"+id, nil, f.member); resp.Code != 204 {
		t.Fatalf("delete: %d %s", resp.Code, resp.Body)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id, nil, f.owner); resp.Code != 404 {
		t.Fatalf("after delete: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/history", nil, f.owner); resp.Code != 404 {
		t.Fatalf("history after delete: %d", resp.Code)
	}
}

func TestListLinksScopingAndPaging(t *testing.T) {
	f := newLinkFixture(t)
	for i := 0; i < 5; i++ {
		f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/team", "title": "team"})
	}
	for i := 0; i < 2; i++ {
		f.a.Do(http.MethodPost, "/api/links", map[string]any{"teamId": f.other, "targetUrl": "https://example.com/other"}, f.owner)
	}
	all := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.owner)
	if all.Code != 200 || all.JSON["total"] != float64(7) || len(all.JSON["items"].([]any)) != 7 {
		t.Fatalf("owner list: %d %s", all.Code, all.Body)
	}
	mine := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.member)
	if mine.JSON["total"] != float64(5) {
		t.Fatalf("member list: %s", mine.Body)
	}
	none := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.outside)
	if none.Code != 200 || none.JSON["total"] != float64(0) {
		t.Fatalf("outsider list: %d %s", none.Code, none.Body)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID, nil, f.stranger); resp.Code != 403 {
		t.Fatalf("stranger list: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&teamId="+f.other, nil, f.member); resp.Code != 403 {
		t.Fatalf("member filtering another team: %d", resp.Code)
	}
	byTeam := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&teamId="+f.other, nil, f.owner)
	if byTeam.JSON["total"] != float64(2) {
		t.Fatalf("team filter: %s", byTeam.Body)
	}
	// paging newest first, no overlap
	seen := map[string]bool{}
	for page := 1; page <= 4; page++ {
		resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&page="+string(rune('0'+page))+"&pageSize=2", nil, f.owner)
		if resp.Code != 200 || resp.JSON["page"] != float64(page) || resp.JSON["pageSize"] != float64(2) {
			t.Fatalf("page %d: %d %s", page, resp.Code, resp.Body)
		}
		for _, it := range resp.JSON["items"].([]any) {
			id := it.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate %s on page %d", id, page)
			}
			seen[id] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("paged %d of 7", len(seen))
	}
	if resp := f.a.Do(http.MethodGet, "/api/links?organizationId="+f.orgID+"&pageSize=9999", nil, f.owner); resp.Code != 400 {
		t.Fatalf("pageSize cap: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links", nil, f.owner); resp.Code != 400 {
		t.Fatalf("missing organizationId: %d", resp.Code)
	}
}
