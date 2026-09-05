package redirect_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/refsdal/snarvei/server/internal/auth"
	"github.com/refsdal/snarvei/server/internal/db/gen"
	"github.com/refsdal/snarvei/server/internal/redirect"
	"github.com/refsdal/snarvei/server/internal/testrig"
)

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestSanitizers(t *testing.T) {
	cases := map[string]string{
		"utm_source=news&token=secret&UTM_Medium=email": "utm_source=news&utm_medium=email",
		"token=secret": "<nil>",
		"":             "<nil>",
		"utm_campaign=" + strings.Repeat("x", 300): "utm_campaign=" + strings.Repeat("x", 200),
		"utm_source=a&utm_source=b":                "utm_source=a&utm_source=b",
	}
	for in, want := range cases {
		if got := str(redirect.SanitizeQueryString(in)); got != want {
			t.Errorf("query %q: got %q want %q", in, got, want)
		}
	}
	refs := map[string]string{
		"https://user:pw@example.com/path?q=1#frag": "https://example.com/path",
		"https://example.com":                       "https://example.com/",
		"not a url":                                 "<nil>",
		"":                                          "<nil>",
	}
	for in, want := range refs {
		if got := str(redirect.SanitizeReferer(in)); got != want {
			t.Errorf("referer %q: got %q want %q", in, got, want)
		}
	}
	if got := str(redirect.SanitizeUserAgent(strings.Repeat("u", 300))); len(got) != 256 {
		t.Errorf("user agent not capped: %d", len(got))
	}
	if redirect.SanitizeUserAgent("") != nil {
		t.Error("empty user agent must be nil")
	}
	// A multibyte rune ("€", 3 bytes) straddling the 256-byte cut must not
	// leave an invalid trailing sequence.
	straddling := strings.Repeat("a", 255) + "€"
	if got := str(redirect.SanitizeUserAgent(straddling)); !utf8.ValidString(got) {
		t.Errorf("truncated user agent is not valid UTF-8: %q", got)
	}
}

func TestRecorderInsertsAndDrains(t *testing.T) {
	rig := testrig.Setup(t)
	q := gen.New(rig.Pool)
	ctx := context.Background()
	// A link needs an org and a team; insert the minimum directly.
	orgID, teamID, linkID := auth.NewID(), auth.NewID(), auth.NewID()
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Acme', 'acme')`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO teams (id, organization_id, name) VALUES ($1, $2, 'Marketing')`, teamID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.Pool.Exec(ctx, `INSERT INTO links (id, organization_id, team_id, slug, target_url) VALUES ($1, $2, $3, 'abc12345', 'https://example.com')`, linkID, orgID, teamID); err != nil {
		t.Fatal(err)
	}

	rec := redirect.NewRecorder(q, slog.Default())
	ua := "Mozilla/5.0"
	for i := 0; i < 3; i++ {
		rec.Record(redirect.ClickEvent{LinkID: linkID, Slug: "abc12345", IPHash: strings.Repeat("a", 64), UserAgent: &ua, Host: "snarvei.test", Path: "/l/abc12345", RedirectStatus: 302})
	}
	if !rec.Drain(5 * time.Second) {
		t.Fatal("drain timed out")
	}
	var n int
	if err := rig.Pool.QueryRow(ctx, `SELECT count(*) FROM click_events WHERE link_id = $1`, linkID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("clicks stored: %d %v", n, err)
	}

	// A failing insert (unknown link) is logged, not fatal, and does not block Drain.
	rec.Record(redirect.ClickEvent{LinkID: "missing", Slug: "x", IPHash: "h", Host: "h", Path: "/l/x", RedirectStatus: 302})
	if !rec.Drain(5 * time.Second) {
		t.Fatal("drain after a failed insert timed out")
	}
}
