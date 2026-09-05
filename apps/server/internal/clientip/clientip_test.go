package clientip

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFromRequest(t *testing.T) {
	cases := []struct {
		name          string
		remote, xff   string
		hops          int
		want          string
	}{
		{"zero hops ignores the header", "10.0.0.1:5555", "1.2.3.4", 0, "10.0.0.1"},
		{"one hop picks the rightmost entry", "10.0.0.1:5555", "9.9.9.9, 1.2.3.4", 1, "1.2.3.4"},
		{"two hops counts back from the right", "10.0.0.1:5555", "9.9.9.9, 1.2.3.4, 172.16.0.1", 2, "1.2.3.4"},
		{"more hops than entries floors at the leftmost", "10.0.0.1:5555", "1.2.3.4, 172.16.0.1", 5, "1.2.3.4"},
		{"hops but no header falls back to the peer", "10.0.0.1:5555", "", 1, "10.0.0.1"},
		{"ipv6 peer keeps its host", "[::1]:5555", "", 0, "::1"},
		{"empty everything is unknown", "", "", 0, "unknown"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := FromRequest(r, c.hops); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestCountry(t *testing.T) {
	cases := []struct {
		header string
		hops   int
		want   string
	}{
		{"NO", 1, "NO"},
		{"no", 1, "NO"},
		{"NO", 0, ""},
		{"XX", 1, ""},
		{"T1", 1, ""},
		{"", 1, ""},
		{"NOR", 1, ""},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		if c.header != "" {
			r.Header.Set("CF-IPCountry", c.header)
		}
		if got := Country(r, c.hops); got != c.want {
			t.Errorf("header %q hops %d: got %q want %q", c.header, c.hops, got, c.want)
		}
	}
}

func TestHasher(t *testing.T) {
	a := NewHasher("", strings.Repeat("s", 32))
	b := NewHasher("", strings.Repeat("s", 32))
	c := NewHasher("pepper", strings.Repeat("s", 32))
	if a.Hash("1.2.3.4") != b.Hash("1.2.3.4") {
		t.Fatal("same secret must give the same hash")
	}
	if a.Hash("1.2.3.4") == c.Hash("1.2.3.4") {
		t.Fatal("a pepper must change the hash")
	}
	if a.Hash("1.2.3.4") == a.Hash("1.2.3.5") {
		t.Fatal("different addresses must differ")
	}
	if h := a.Hash("1.2.3.4"); len(h) != 64 || strings.ContainsAny(h, "1234.") && strings.Contains(h, "1.2.3.4") {
		t.Fatalf("hash must be 64 hex chars and not contain the address: %q", h)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if a.Extractor(1)(r) != a.Hash("1.2.3.4") {
		t.Fatal("extractor must hash the trusted client address")
	}
	if a.Extractor(0)(r) != a.Hash("10.0.0.1") {
		t.Fatal("extractor with zero hops must hash the peer")
	}
}
