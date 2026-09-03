package config

import "testing"

func TestValidateRejectsEmptySecrets(t *testing.T) {
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("empty configuration accepted")
	}
}

func TestFromEnvRequiresIndependentEditorSecrets(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "admin-secret-long-enough")
	t.Setenv("HERMES_API_KEY", "hermes-secret-long-enough")
	t.Setenv("EDITOR_PASSWORD", "")
	t.Setenv("SESSION_SECRET", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("missing editor/session secrets accepted")
	}
}

func TestFromEnvLoadsSecureEditorConfig(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "admin-secret-long-enough")
	t.Setenv("HERMES_API_KEY", "hermes-secret-long-enough")
	t.Setenv("EDITOR_USER", "editor")
	t.Setenv("EDITOR_PASSWORD", "editor-password-long-enough")
	t.Setenv("SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("ADMIN_COOKIE_PATH", "/oddity/admin")
	t.Setenv("PUBLIC_BASE_URL", "https://example.test/oddity")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SecureCookies || cfg.AdminCookiePath != "/oddity/admin" {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestFromEnvRejectsSharedSessionSecret(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "admin-secret-long-enough")
	t.Setenv("HERMES_API_KEY", "hermes-secret-long-enough")
	t.Setenv("EDITOR_PASSWORD", "editor-password-long-enough")
	t.Setenv("SESSION_SECRET", "editor-password-long-enough")
	if _, err := FromEnv(); err == nil {
		t.Fatal("shared session secret accepted")
	}
}

func TestFromEnvRejectsEqualAdminAndHermesKeys(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "same-secret")
	t.Setenv("HERMES_API_KEY", "same-secret")
	if _, err := FromEnv(); err == nil {
		t.Fatal("equal admin and Hermes keys accepted")
	}
}
