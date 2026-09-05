package links

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		s := GenerateSlug()
		if len(s) != SlugLength {
			t.Fatalf("length %d: %q", len(s), s)
		}
		if strings.ContainsAny(s, "0OIl1") {
			t.Fatalf("ambiguous character in %q", s)
		}
		if !regexp.MustCompile(`^[A-Za-z2-9]+$`).MatchString(s) {
			t.Fatalf("unexpected character in %q", s)
		}
		seen[s] = true
	}
	if len(seen) < 495 {
		t.Fatalf("slugs are not random enough: %d distinct of 500", len(seen))
	}
}

func TestNormalizeCustomSlug(t *testing.T) {
	ok := map[string]string{"  Summer-2026 ": "summer-2026", "abc": "abc", "a1-b2-c3": "a1-b2-c3"}
	for in, want := range ok {
		got, err := NormalizeCustomSlug(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "ab", "Hello World!", "-lead", "trail-", "double--hyphen", strings.Repeat("a", 65), "under_score", "ünïcode"} {
		if _, err := NormalizeCustomSlug(bad); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidateTargetURL(t *testing.T) {
	for _, good := range []string{"https://example.com", "http://example.com/path?query=1#frag", "https://sub.example.co.uk:8443/a/b", "https://xn--bcher-kva.example/", "  https://example.com/x  "} {
		if _, err := ValidateTargetURL(good); err != nil {
			t.Errorf("%q rejected: %v", good, err)
		}
	}
	if got, _ := ValidateTargetURL("  https://example.com/x  "); got != "https://example.com/x" {
		t.Errorf("not trimmed: %q", got)
	}
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,<script>alert(1)</script>", "file:///etc/passwd", "intent://scan/#Intent;scheme=zxing;end", "mailto:someone@example.com", "ftp://example.com/file", "//example.com/path", "https://user:pass@example.com/", "example.com/path", "not a url", "", "   ", "https://example.com/" + strings.Repeat("a", 2048)} {
		if _, err := ValidateTargetURL(bad); !errors.Is(err, ErrInvalidTargetURL) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidRedirectStatus(t *testing.T) {
	for s, want := range map[int]bool{301: true, 302: true, 307: true, 308: false, 200: false, 0: false} {
		if ValidRedirectStatus(s) != want {
			t.Errorf("%d", s)
		}
	}
}
