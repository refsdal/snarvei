// Package links holds the pure rules for short links: slug generation, custom
// slug normalisation and target-URL validation. No I/O.
package links

import (
	"crypto/rand"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Alphabet omits 0/O, I/l/1 so a printed slug cannot be misread.
const Alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// SlugLength is the generated slug length.
const SlugLength = 8

const (
	minSlug          = 3
	maxSlug          = 64
	maxTargetURLSize = 2048
)

var (
	ErrInvalidSlug      = errors.New("links: slug may only contain lowercase letters, digits and single hyphens (3-64 characters)")
	ErrInvalidTargetURL = errors.New("links: target URL must be an absolute http(s) URL without credentials (at most 2048 characters)")
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// GenerateSlug returns a random slug; the modulo bias over a 57-letter
// alphabet is negligible for this purpose.
func GenerateSlug() string {
	buf := make([]byte, SlugLength)
	if _, err := rand.Read(buf); err != nil {
		panic("links: crypto/rand failed: " + err.Error())
	}
	out := make([]byte, SlugLength)
	for i, b := range buf {
		out[i] = Alphabet[int(b)%len(Alphabet)]
	}
	return string(out)
}

// NormalizeCustomSlug trims, lower-cases and validates a user-chosen slug.
func NormalizeCustomSlug(raw string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(raw))
	if len(slug) < minSlug || len(slug) > maxSlug || !slugPattern.MatchString(slug) {
		return "", ErrInvalidSlug
	}
	return slug, nil
}

// ValidateTargetURL trims and checks a redirect target.
func ValidateTargetURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxTargetURLSize {
		return "", ErrInvalidTargetURL
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", ErrInvalidTargetURL
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", ErrInvalidTargetURL
	}
	return value, nil
}

// ValidRedirectStatus reports whether s is one of the supported statuses.
func ValidRedirectStatus(s int) bool { return s == 301 || s == 302 || s == 307 }
