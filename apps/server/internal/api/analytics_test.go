package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestAnalytics(t *testing.T) {
	f := newLinkFixture(t)
	created := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com"})
	id := created.JSON["id"].(string)
	ctx := context.Background()
	insert := func(age time.Duration, ip, referer, country string) {
		t.Helper()
		var ref, ctry *string
		if referer != "" {
			ref = &referer
		}
		if country != "" {
			ctry = &country
		}
		if _, err := f.a.Rig.Pool.Exec(ctx, `INSERT INTO click_events (id, link_id, clicked_at, ip_hash, referer, country, host, path, redirect_status_used) VALUES (gen_random_uuid()::text, $1, now() - $2::interval, $3, $4, $5, 'h', '/l/x', 302)`,
			id, age.String(), ip, ref, ctry); err != nil {
			t.Fatal(err)
		}
	}
	insert(time.Hour, "ip1", "https://news.example/a", "NO")
	insert(2*time.Hour, "ip1", "https://news.example/a", "NO")
	insert(48*time.Hour, "ip2", "", "SE")
	insert(40*24*time.Hour, "ip3", "https://old.example/", "DE") // outside the default 30 days

	resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics", nil, f.member)
	if resp.Code != 200 || resp.JSON["totalClicks"] != float64(3) || resp.JSON["uniqueVisitorApproximation"] != float64(2) {
		t.Fatalf("analytics: %d %s", resp.Code, resp.Body)
	}
	refs := resp.JSON["topReferrers"].([]any)
	if refs[0].(map[string]any)["referer"] != "https://news.example/a" || refs[0].(map[string]any)["clicks"] != float64(2) {
		t.Fatalf("referrers: %s", resp.Body)
	}
	countries := resp.JSON["topCountries"].([]any)
	if countries[0].(map[string]any)["country"] != "NO" {
		t.Fatalf("countries: %s", resp.Body)
	}
	days := resp.JSON["clicksByDay"].([]any)
	sum := 0.0
	for _, d := range days {
		sum += d.(map[string]any)["clicks"].(float64)
	}
	if sum != 3 {
		t.Fatalf("clicksByDay: %s", resp.Body)
	}
	rng := resp.JSON["range"].(map[string]any)
	from, _ := time.Parse(time.RFC3339, rng["from"].(string))
	if time.Since(from) < 29*24*time.Hour || time.Since(from) > 31*24*time.Hour {
		t.Fatalf("range.from: %v", from)
	}

	wide := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics?days=365", nil, f.owner)
	if wide.JSON["totalClicks"] != float64(4) {
		t.Fatalf("365 days: %s", wide.Body)
	}
	for _, bad := range []string{"0", "366", "x"} {
		if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics?days="+bad, nil, f.owner); resp.Code != 400 {
			t.Errorf("days=%s: %d", bad, resp.Code)
		}
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics", nil, f.outside); resp.Code != 403 {
		t.Fatalf("outsider: %d", resp.Code)
	}
	if resp := f.a.Do(http.MethodGet, "/api/links/"+id+"/analytics", nil, f.stranger); resp.Code != 404 {
		t.Fatalf("stranger: %d", resp.Code)
	}
}
