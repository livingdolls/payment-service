package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppEnv      string
	HTTPPort    string
	DatabaseURL string

	XenditSecretKey    string
	XenditBaseURL      string
	XenditWebhookToken string

	AdminAPIToken string
	AdminAPIActor string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		HTTPPort:    getEnv("HTTP_PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),

		XenditSecretKey:    os.Getenv("XENDIT_SECRET_KEY"),
		XenditBaseURL:      getEnv("XENDIT_BASE_URL", "https://api.xendit.co"),
		XenditWebhookToken: os.Getenv("XENDIT_WEBHOOK_TOKEN"),
		AdminAPIToken:      os.Getenv("ADMIN_API_TOKEN"),
		AdminAPIActor:      getEnv("ADMIN_API_ACTOR", "local-admin-api"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	if cfg.XenditSecretKey == "" {
		return nil, fmt.Errorf("XENDIT_SECRET_KEY is required")
	}

	if cfg.XenditWebhookToken == "" {
		return nil, fmt.Errorf("XENDIT_WEBHOOK_TOKEN is required")
	}

	if len(cfg.AdminAPIToken) < 32 {
		return nil, fmt.Errorf("ADMIN_API_TOKEN must contain a sufficiently long secret")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}
