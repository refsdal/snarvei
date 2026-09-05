package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/refsdal/snarvei/server/internal/testrig"
)

func follow(a *testrig.AppRig, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "203.0.113.5:1234"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return a.DoRaw(req)
}

func TestRedirectAndClickRecording(t *testing.T) {
	f := newLinkFixture(t)
	created := f.create(t, f.owner, map[string]any{"targetUrl": "https://example.com/landing", "redirectStatus": 307})
	slug, id := created.JSON["slug"].(string), created.JSON["id"].(string)

	rec := follow(f.a, "/l/"+slug+"?utm_source=news&token=secret", map[string]string{"User-Agent": "UA/1.0", "Referer": "https://ref.example/page?x=1", "CF-IPCountry": "NO"})
	if rec.Code != 307 || rec.Header().Get("Location") != "https://example.com/landing" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redirect: %d %q %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Cache-Control"))
	}
	if !f.a.Clicks.Drain(5 * time.Second) {
		t.Fatal("drain")
	}
	var ipHash, ua, ref, qs, host, path string
	var country *string
	var status int
	err := f.a.Rig.Pool.QueryRow(context.Background(), `SELECT ip_hash, COALESCE(user_agent,''), COALESCE(referer,''), COALESCE(query_string,''), country, host, path, redirect_status_used FROM click_events WHERE link_id = $1`, id).
		Scan(&ipHash, &ua, &ref, &qs, &country, &host, &path, &status)
	if err != nil {
		t.Fatal(err)
	}
	if len(ipHash) != 64 || ipHash == "203.0.113.5" || ua != "UA/1.0" || ref != "https://ref.example/page" || qs != "utm_source=news" || path != "/l/"+slug || status != 307 {
		t.Fatalf("click row: %q %q %q %q %q %d", ipHash, ua, ref, qs, path, status)
	}
	if country != nil { // TrustedProxyHops is 0 in the rig: CF-IPCountry is untrusted
		t.Fatalf("country must be null when no proxy is trusted: %v", *country)
	}

	miss := follow(f.a, "/l/doesnotexist", nil)
	if miss.Code != 404 || miss.Header().Get("Cache-Control") != "no-store" || miss.Body.String() != "Link not found" || miss.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("miss: %d %q %q", miss.Code, miss.Header().Get("Cache-Control"), miss.Body.String())
	}
	f.a.Do(http.MethodPatch, "/api/links/"+id, map[string]any{"isActive": false}, f.owner)
	if rec := follow(f.a, "/l/"+slug, nil); rec.Code != 404 {
		t.Fatalf("inactive: %d", rec.Code)
	}
	f.a.Clicks.Drain(5 * time.Second)
	var n int
	_ = f.a.Rig.Pool.QueryRow(context.Background(), `SELECT count(*) FROM click_events WHERE link_id = $1`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("inactive link must record no click: %d", n)
	}
	f.a.Do(http.MethodDelete, "/api/links/"+id, nil, f.owner)
	_ = f.a.Rig.Pool.QueryRow(context.Background(), `SELECT count(*) FROM click_events WHERE link_id = $1`, id).Scan(&n)
	if n != 0 {
		t.Fatal("deleting the link must delete its clicks")
	}
}

func TestRedirectIsRateLimited(t *testing.T) {
	f := newLinkFixture(t)
	for i := 0; i <= 100; i++ {
		rec := follow(f.a, "/l/nothing", nil)
		if i < 100 && rec.Code != 404 {
			t.Fatalf("hit %d: %d", i, rec.Code)
		}
		if i == 100 && (rec.Code != 429 || rec.Header().Get("Retry-After") == "") {
			t.Fatalf("hit 101: %d", rec.Code)
		}
	}
}
