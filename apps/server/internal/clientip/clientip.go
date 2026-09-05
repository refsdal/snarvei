// Package clientip derives the caller's address from a request behind N
// trusted proxies, the country Cloudflare reports, and the keyed digest that
// is the only form of the address Snarvei ever stores.
package clientip

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// FromRequest returns the client address. With trustedHops == 0 the peer
// address is the client; with N > 0 the N-th entry from the right of
// X-Forwarded-For is (Cloudflare proxied DNS = 1). Never empty.
func FromRequest(r *http.Request, trustedHops int) string {
	peer := hostOnly(r.RemoteAddr)
	if trustedHops <= 0 {
		return orUnknown(peer)
	}
	var chain []string
	for _, part := range strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",") {
		if p := strings.TrimSpace(part); p != "" {
			chain = append(chain, p)
		}
	}
	if len(chain) == 0 {
		return orUnknown(peer)
	}
	index := len(chain) - trustedHops
	if index < 0 {
		index = 0
	}
	return orUnknown(chain[index])
}

// Country returns Cloudflare's two-letter country code when a proxy is
// trusted, upper-cased; "" for untrusted requests, absent headers and the
// XX/T1 placeholders.
func Country(r *http.Request, trustedHops int) string {
	if trustedHops <= 0 {
		return ""
	}
	code := strings.ToUpper(strings.TrimSpace(r.Header.Get("CF-IPCountry")))
	if len(code) != 2 || code == "XX" || code == "T1" {
		return ""
	}
	return code
}

// Hasher produces keyed digests of addresses.
type Hasher struct{ key []byte }

// NewHasher uses pepper when set, otherwise a key derived from authSecret
// with its own domain separator so it can never equal the signing secret.
func NewHasher(pepper, authSecret string) *Hasher {
	if pepper != "" {
		return &Hasher{key: []byte(pepper)}
	}
	mac := hmac.New(sha256.New, []byte(authSecret))
	mac.Write([]byte("snarvei:ip-hash"))
	return &Hasher{key: mac.Sum(nil)}
}

// Hash returns the hex HMAC-SHA256 of ip.
func (h *Hasher) Hash(ip string) string {
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

// Extractor adapts the hasher to the func(*http.Request) string shape Limen
// wants for session metadata and rate-limit keys.
func (h *Hasher) Extractor(trustedHops int) func(*http.Request) string {
	return func(r *http.Request) string { return h.Hash(FromRequest(r, trustedHops)) }
}

func hostOnly(address string) string {
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return address
}

func orUnknown(address string) string {
	if address == "" {
		return "unknown"
	}
	return address
}
