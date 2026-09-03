package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr              string
	DatabasePath      string
	AdminAPIKey       string
	HermesAPIKey      string
	StorageDriver     string
	LocalStoragePath  string
	PublicBaseURL     string
	MaxUploadBytes    int64
	R2Endpoint        string
	R2Bucket          string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2PublicBaseURL   string
	EditorUser        string
	EditorPassword    string
	SessionSecret     string
	AdminCookiePath   string
	SecureCookies     bool
}

func FromEnv() (Config, error) {
	cfg := Config{
		Addr:              env("ODDITY_ADDR", "127.0.0.1:8890"),
		DatabasePath:      env("ODDITY_DB_PATH", "./data/oddity.db"),
		AdminAPIKey:       os.Getenv("ADMIN_API_KEY"),
		HermesAPIKey:      os.Getenv("HERMES_API_KEY"),
		StorageDriver:     env("STORAGE_DRIVER", "local"),
		LocalStoragePath:  env("LOCAL_STORAGE_PATH", "./data/media"),
		PublicBaseURL:     env("PUBLIC_BASE_URL", "http://127.0.0.1:8890"),
		MaxUploadBytes:    10 << 20,
		R2Endpoint:        os.Getenv("R2_ENDPOINT"),
		R2Bucket:          os.Getenv("R2_BUCKET"),
		R2AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2PublicBaseURL:   os.Getenv("R2_PUBLIC_BASE_URL"),
		EditorUser:        env("EDITOR_USER", "editor"),
		EditorPassword:    os.Getenv("EDITOR_PASSWORD"),
		SessionSecret:     os.Getenv("SESSION_SECRET"),
		AdminCookiePath:   env("ADMIN_COOKIE_PATH", "/admin"),
		SecureCookies:     strings.HasPrefix(env("PUBLIC_BASE_URL", "http://127.0.0.1:8890"), "https://"),
	}
	if raw := os.Getenv("MAX_UPLOAD_BYTES"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES invalid")
		}
		cfg.MaxUploadBytes = value
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.StorageDriver == "r2" && (cfg.R2Endpoint == "" || cfg.R2Bucket == "" || cfg.R2AccessKeyID == "" || cfg.R2SecretAccessKey == "" || cfg.R2PublicBaseURL == "") {
		return fmt.Errorf("R2 configuration incomplete")
	}
	if cfg.AdminAPIKey == "" || cfg.HermesAPIKey == "" {
		return fmt.Errorf("ADMIN_API_KEY and HERMES_API_KEY are required")
	}
	if cfg.AdminAPIKey == cfg.HermesAPIKey {
		return fmt.Errorf("ADMIN_API_KEY and HERMES_API_KEY must differ")
	}
	if len(cfg.EditorPassword) < 12 {
		return fmt.Errorf("EDITOR_PASSWORD must contain at least 12 characters")
	}
	if cfg.EditorPassword == cfg.AdminAPIKey || cfg.EditorPassword == cfg.HermesAPIKey {
		return fmt.Errorf("EDITOR_PASSWORD must differ from API keys")
	}
	if len(cfg.SessionSecret) < 32 {
		return fmt.Errorf("SESSION_SECRET must contain at least 32 characters")
	}
	if cfg.SessionSecret == cfg.EditorPassword || cfg.SessionSecret == cfg.AdminAPIKey || cfg.SessionSecret == cfg.HermesAPIKey {
		return fmt.Errorf("SESSION_SECRET must differ from all credentials")
	}
	if cfg.EditorUser == "" || strings.Contains(cfg.EditorUser, "|") {
		return fmt.Errorf("EDITOR_USER is invalid")
	}
	if !strings.HasPrefix(cfg.AdminCookiePath, "/") {
		return fmt.Errorf("ADMIN_COOKIE_PATH must start with /")
	}
	return nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
