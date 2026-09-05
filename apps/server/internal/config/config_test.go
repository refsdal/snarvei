package config

import (
	"strings"
	"testing"
)

func minimal() map[string]string {
	return map[string]string{
		"DATABASE_URL":    "postgres://snarvei:snarvei@localhost:5432/snarvei",
		"APP_URL":         "https://snarvei.example.com",
		"AUTH_SECRET":     strings.Repeat("s", 32),
		"STORAGE_DRIVER":  "fs",
		"STORAGE_FS_PATH": "/data",
	}
}

func TestLoadMinimalAppliesDefaults(t *testing.T) {
	cfg, err := Load(minimal())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 || cfg.AppName != "Snarvei" || cfg.TrustedProxyHops != 0 || !cfg.OpenSignup ||
		cfg.MigrationLockKey != 1935762089 || cfg.LogLevel != "info" || cfg.E2ETestHooks {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if !cfg.Secure() {
		t.Fatal("https APP_URL must be Secure")
	}
	if cfg.EmailEnabled() {
		t.Fatal("email must be off without SMTP_*")
	}
}

func TestLoadReportsEveryMissingRequiredVariable(t *testing.T) {
	_, err := Load(map[string]string{})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DATABASE_URL", "APP_URL", "AUTH_SECRET", "STORAGE_DRIVER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestLoadRejectsShortSecretAndBadURL(t *testing.T) {
	env := minimal()
	env["AUTH_SECRET"] = "short"
	env["APP_URL"] = "not-a-url"
	_, err := Load(env)
	if err == nil || !strings.Contains(err.Error(), "AUTH_SECRET") || !strings.Contains(err.Error(), "APP_URL") {
		t.Fatalf("expected both problems, got: %v", err)
	}
}

func TestLoadS3RequiresAllFour(t *testing.T) {
	env := minimal()
	env["STORAGE_DRIVER"] = "s3"
	delete(env, "STORAGE_FS_PATH")
	env["S3_BUCKET"] = "b"
	_, err := Load(env)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"S3_ENDPOINT", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
	env["S3_ENDPOINT"] = "https://s3.example.com"
	env["S3_ACCESS_KEY_ID"] = "k"
	env["S3_SECRET_ACCESS_KEY"] = "s"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.S3Region != "auto" {
		t.Fatalf("S3_REGION default = %q, want auto", cfg.S3Region)
	}
}

func TestLoadSMTPIsAllOrNothing(t *testing.T) {
	env := minimal()
	env["SMTP_HOST"] = "smtp.example.com"
	_, err := Load(env)
	if err == nil || !strings.Contains(err.Error(), "SMTP_PORT") || !strings.Contains(err.Error(), "EMAIL_FROM") {
		t.Fatalf("partial SMTP config must fail naming the missing ones, got: %v", err)
	}
	env["SMTP_PORT"] = "587"
	env["SMTP_USERNAME"] = "u"
	env["SMTP_PASSWORD"] = "p"
	env["EMAIL_FROM"] = "Snarvei <no-reply@example.com>"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.EmailEnabled() || cfg.SMTPPort != 587 {
		t.Fatalf("email should be enabled: %+v", cfg)
	}
}

func TestLoadSwitches(t *testing.T) {
	env := minimal()
	env["OPEN_SIGNUP"] = "0"
	env["PORT"] = "8080"
	env["TRUSTED_PROXY_HOPS"] = "2"
	env["LOG_LEVEL"] = "debug"
	env["MIGRATION_LOCK_KEY"] = "42"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OpenSignup || cfg.Port != 8080 || cfg.TrustedProxyHops != 2 || cfg.LogLevel != "debug" || cfg.MigrationLockKey != 42 {
		t.Fatalf("switches not applied: %+v", cfg)
	}

	env["OPEN_SIGNUP"] = "yes"
	env["PORT"] = "-1"
	env["LOG_LEVEL"] = "loud"
	if _, err := Load(env); err == nil || !strings.Contains(err.Error(), "OPEN_SIGNUP") ||
		!strings.Contains(err.Error(), "PORT") || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("expected OPEN_SIGNUP, PORT and LOG_LEVEL problems, got: %v", err)
	}
}

func TestE2ETestHooksOnlyOnLoopback(t *testing.T) {
	env := minimal()
	env["E2E_TEST_HOOKS"] = "1"
	if _, err := Load(env); err == nil || !strings.Contains(err.Error(), "E2E_TEST_HOOKS") {
		t.Fatalf("hooks on a public APP_URL must fail, got: %v", err)
	}
	env["APP_URL"] = "http://127.0.0.1:3300"
	cfg, err := Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.E2ETestHooks || cfg.Secure() {
		t.Fatalf("expected hooks on and Secure off: %+v", cfg)
	}
}

func TestDisabledSubsystems(t *testing.T) {
	cfg, err := Load(minimal())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := strings.Join(cfg.DisabledSubsystems(), ","); got != "email" {
		t.Fatalf("DisabledSubsystems = %q, want email", got)
	}
}
