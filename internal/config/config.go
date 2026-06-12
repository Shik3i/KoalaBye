package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const insecureDefaultSecret = "change-me-long-random-secret"

type Config struct {
	BaseURL              string
	ListenAddr           string
	DatabasePath         string
	Secret               string
	Mode                 string
	RegistrationEnabled  bool
	InviteOnly           bool
	SecureCookies        bool
	InstanceName         string
	BootstrapUsername    string
	BootstrapPassword    string
	BootstrapDisplayName string
}

func Load() (Config, error) {
	cfg := Config{
		BaseURL:              env("KOALABYE_BASE_URL", "http://localhost:8080"),
		ListenAddr:           env("KOALABYE_LISTEN_ADDR", ":8080"),
		DatabasePath:         env("KOALABYE_DATABASE_PATH", "./data/koalabye.db"),
		Secret:               env("KOALABYE_SECRET", insecureDefaultSecret),
		Mode:                 env("KOALABYE_MODE", "selfhost"),
		RegistrationEnabled:  envBool("KOALABYE_REGISTRATION_ENABLED", false),
		InviteOnly:           envBool("KOALABYE_INVITE_ONLY", true),
		SecureCookies:        envBool("KOALABYE_SECURE_COOKIES", false),
		InstanceName:         env("KOALABYE_INSTANCE_NAME", "KoalaBye"),
		BootstrapUsername:    strings.TrimSpace(os.Getenv("KOALABYE_BOOTSTRAP_ADMIN_USERNAME")),
		BootstrapPassword:    os.Getenv("KOALABYE_BOOTSTRAP_ADMIN_PASSWORD"),
		BootstrapDisplayName: strings.TrimSpace(os.Getenv("KOALABYE_BOOTSTRAP_ADMIN_DISPLAY_NAME")),
	}

	if cfg.Mode != "selfhost" && cfg.Mode != "cloud" {
		return Config{}, fmt.Errorf("KOALABYE_MODE must be selfhost or cloud, got %q", cfg.Mode)
	}
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return Config{}, fmt.Errorf("invalid KOALABYE_BASE_URL: %w", err)
	}
	if cfg.Secret == "" {
		return Config{}, errors.New("KOALABYE_SECRET is required")
	}
	if cfg.Secret == insecureDefaultSecret {
		if cfg.isLocalDevelopment() {
			slog.Warn("using insecure development secret; set KOALABYE_SECRET before deployment")
		} else {
			return Config{}, errors.New("KOALABYE_SECRET must be changed from the insecure default")
		}
	}
	if len(cfg.Secret) < 32 && !cfg.isLocalDevelopment() {
		return Config{}, errors.New("KOALABYE_SECRET must be at least 32 characters outside local development")
	}
	if (cfg.BootstrapUsername == "") != (cfg.BootstrapPassword == "") {
		return Config{}, errors.New("bootstrap username and password must be provided together")
	}
	if cfg.BootstrapPassword != "" && len(cfg.BootstrapPassword) < 12 {
		return Config{}, errors.New("bootstrap password must be at least 12 characters")
	}
	return cfg, nil
}

func (c Config) isLocalDevelopment() bool {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return !c.SecureCookies && (host == "localhost" || host == "127.0.0.1" || host == "::1")
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		slog.Warn("invalid boolean environment value; using fallback", "key", key)
		return fallback
	}
	return value
}
