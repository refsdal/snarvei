// Package config loads and validates the process configuration from
// environment variables: parse once at startup, report EVERY problem at
// once, so a misconfigured container crash-loops with a list rather than
// one restart per mistake. Nothing outside cmd/snarvei calls FromOS.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is the validated process configuration. Every field is populated
// by Load; there is no lazy or partial state.
type Config struct {
	DatabaseURL string
	AppURL      string
	AuthSecret  string

	StorageDriver     string // "fs" | "s3"
	StorageFSPath     string
	S3Bucket          string
	S3Endpoint        string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Region          string

	Port             int
	AppName          string
	TrustedProxyHops int
	OpenSignup       bool
	IPHashPepper     string

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	EmailFrom    string

	MigrationLockKey int64
	LogLevel         string
	E2ETestHooks     bool
}

type problems struct{ list []string }

func (p *problems) add(field, message string) {
	p.list = append(p.list, fmt.Sprintf("%s: %s", field, message))
}

func (p *problems) require(env map[string]string, field string) (string, bool) {
	v := strings.TrimSpace(env[field])
	if v == "" {
		p.add(field, "is required")
		return "", false
	}
	return v, true
}

func isAbsoluteHTTPURL(v string) bool {
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func isLoopbackURL(v string) bool {
	u, err := url.Parse(v)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (p *problems) intOr(env map[string]string, field string, def int, min int) int {
	v := strings.TrimSpace(env[field])
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.add(field, "must be an integer")
		return def
	}
	if n < min {
		p.add(field, fmt.Sprintf("must be at least %d", min))
		return def
	}
	return n
}

func (p *problems) boolOr(env map[string]string, field string, def bool) bool {
	switch strings.TrimSpace(env[field]) {
	case "":
		return def
	case "0":
		return false
	case "1":
		return true
	default:
		p.add(field, `must be "0" or "1"`)
		return def
	}
}

// Load parses and validates a plain string map (the shape a real environ
// and a test fixture share). It reports every invalid or missing field in
// one error.
func Load(env map[string]string) (*Config, error) {
	p := &problems{}
	cfg := &Config{}

	if v, ok := p.require(env, "DATABASE_URL"); ok {
		cfg.DatabaseURL = v
	}
	if v, ok := p.require(env, "APP_URL"); ok {
		if !isAbsoluteHTTPURL(v) {
			p.add("APP_URL", "must be an absolute http(s) URL")
		} else {
			cfg.AppURL = strings.TrimRight(v, "/")
		}
	}
	if v, ok := p.require(env, "AUTH_SECRET"); ok {
		if len(v) < 32 {
			p.add("AUTH_SECRET", "must be at least 32 bytes")
		} else {
			cfg.AuthSecret = v
		}
	}

	if driver, ok := p.require(env, "STORAGE_DRIVER"); ok {
		switch driver {
		case "fs":
			cfg.StorageDriver = driver
			if v, ok := p.require(env, "STORAGE_FS_PATH"); ok {
				cfg.StorageFSPath = v
			}
		case "s3":
			cfg.StorageDriver = driver
			if v, ok := p.require(env, "S3_BUCKET"); ok {
				cfg.S3Bucket = v
			}
			if v, ok := p.require(env, "S3_ENDPOINT"); ok {
				if !isAbsoluteHTTPURL(v) {
					p.add("S3_ENDPOINT", "must be an absolute http(s) URL")
				} else {
					cfg.S3Endpoint = v
				}
			}
			if v, ok := p.require(env, "S3_ACCESS_KEY_ID"); ok {
				cfg.S3AccessKeyID = v
			}
			if v, ok := p.require(env, "S3_SECRET_ACCESS_KEY"); ok {
				cfg.S3SecretAccessKey = v
			}
			cfg.S3Region = "auto"
			if v := strings.TrimSpace(env["S3_REGION"]); v != "" {
				cfg.S3Region = v
			}
		default:
			p.add("STORAGE_DRIVER", `must be "fs" or "s3"`)
		}
	}

	cfg.Port = p.intOr(env, "PORT", 3000, 1)
	cfg.AppName = "Snarvei"
	if v := strings.TrimSpace(env["APP_NAME"]); v != "" {
		cfg.AppName = v
	}
	cfg.TrustedProxyHops = p.intOr(env, "TRUSTED_PROXY_HOPS", 0, 0)
	cfg.OpenSignup = p.boolOr(env, "OPEN_SIGNUP", true)
	cfg.IPHashPepper = env["IP_HASH_PEPPER"]

	// SMTP: all five or none. Half a mail configuration is a misconfiguration,
	// not "email off".
	smtpFields := []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "EMAIL_FROM"}
	present := 0
	for _, f := range smtpFields {
		if strings.TrimSpace(env[f]) != "" {
			present++
		}
	}
	if present > 0 {
		for _, f := range smtpFields {
			if strings.TrimSpace(env[f]) == "" {
				p.add(f, "is required when any SMTP_* variable is set")
			}
		}
		cfg.SMTPHost = strings.TrimSpace(env["SMTP_HOST"])
		cfg.SMTPPort = p.intOr(env, "SMTP_PORT", 0, 1)
		cfg.SMTPUsername = env["SMTP_USERNAME"]
		cfg.SMTPPassword = env["SMTP_PASSWORD"]
		cfg.EmailFrom = strings.TrimSpace(env["EMAIL_FROM"])
	}

	cfg.MigrationLockKey = 1935762089
	if v := strings.TrimSpace(env["MIGRATION_LOCK_KEY"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			p.add("MIGRATION_LOCK_KEY", "must be an integer")
		} else {
			cfg.MigrationLockKey = n
		}
	}

	cfg.LogLevel = "info"
	if v := strings.TrimSpace(env["LOG_LEVEL"]); v != "" {
		switch v {
		case "debug", "info", "warn", "error":
			cfg.LogLevel = v
		default:
			p.add("LOG_LEVEL", `must be one of "debug", "info", "warn", "error"`)
		}
	}

	cfg.E2ETestHooks = p.boolOr(env, "E2E_TEST_HOOKS", false)
	if cfg.E2ETestHooks && cfg.AppURL != "" && !isLoopbackURL(cfg.AppURL) {
		p.add("E2E_TEST_HOOKS", "may only be enabled when APP_URL is a loopback origin")
	}

	if len(p.list) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  %s", strings.Join(p.list, "\n  "))
	}
	return cfg, nil
}

// FromOS loads configuration from the real environment.
func FromOS() (*Config, error) {
	env := make(map[string]string, len(os.Environ()))
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return Load(env)
}

// EmailEnabled reports whether transactional email is configured.
func (c *Config) EmailEnabled() bool { return c.SMTPHost != "" }

// Secure reports whether APP_URL is https, which decides cookie Secure flags.
func (c *Config) Secure() bool { return strings.HasPrefix(c.AppURL, "https://") }

// DisabledSubsystems names optional subsystems that are off, for the boot log.
func (c *Config) DisabledSubsystems() []string {
	var off []string
	if !c.EmailEnabled() {
		off = append(off, "email")
	}
	return off
}
