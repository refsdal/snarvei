// Package redirect serves GET /l/{slug}'s click side: data-minimised click
// events and the async recorder that stores them after the redirect is sent.
package redirect

import (
	"net/url"
	"strings"
)

const (
	maxUserAgent = 256
	maxUTMValue  = 200
)

// SanitizeQueryString keeps only utm_* parameters (keys lower-cased, values
// capped), in their original order. Short links travel in campaigns whose
// query strings routinely carry personal data or tokens.
func SanitizeQueryString(raw string) *string {
	if raw == "" {
		return nil
	}
	var kept []string
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		k, err := url.QueryUnescape(key)
		if err != nil {
			continue
		}
		k = strings.ToLower(k)
		if !isUTMKey(k) {
			continue
		}
		v, err := url.QueryUnescape(value)
		if err != nil {
			continue
		}
		if len(v) > maxUTMValue {
			v = v[:maxUTMValue]
		}
		kept = append(kept, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	if len(kept) == 0 {
		return nil
	}
	out := strings.Join(kept, "&")
	return &out
}

func isUTMKey(k string) bool {
	if !strings.HasPrefix(k, "utm_") || len(k) == len("utm_") {
		return false
	}
	for _, c := range k[len("utm_"):] {
		if (c < 'a' || c > 'z') && c != '_' {
			return false
		}
	}
	return true
}

// SanitizeReferer reduces a referer to origin + path.
func SanitizeReferer(raw string) *string {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	out := u.Scheme + "://" + u.Host + path
	return &out
}

// SanitizeUserAgent caps the user agent. Truncation is byte-based, so
// ToValidUTF8 drops any rune left split at the boundary rather than letting
// the invalid tail reach Postgres (which would reject the whole insert).
func SanitizeUserAgent(raw string) *string {
	if raw == "" {
		return nil
	}
	if len(raw) > maxUserAgent {
		raw = strings.ToValidUTF8(raw[:maxUserAgent], "")
	}
	return &raw
}
