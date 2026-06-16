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
	BaseURL                            string
	ListenAddr                         string
	DatabasePath                       string
	Secret                             string
	Mode                               string
	RegistrationEnabled                bool
	InviteOnly                         bool
	InviteRegistrationEnabled          bool
	SecureCookies                      bool
	InstanceName                       string
	InstanceSourceURL                  string
	BootstrapUsername                  string
	BootstrapPassword                  string
	BootstrapDisplayName               string
	DefaultMaxOrganizationsPerUser     int
	DefaultMaxCampaignsPerOrg          int
	DefaultMaxMembersPerOrg            int
	DefaultMaxActiveInvitesPerOrg      int
	DefaultMaxMonthlyVisitsPerOrg      int
	DefaultMaxMonthlySubmissionsPerOrg int
}

func Load() (Config, error) {
	cfg := Config{
		BaseURL:                            env("KOALABYE_BASE_URL", "http://localhost:8080"),
		ListenAddr:                         env("KOALABYE_LISTEN_ADDR", ":8080"),
		DatabasePath:                       env("KOALABYE_DATABASE_PATH", "./data/koalabye.db"),
		Secret:                             env("KOALABYE_SECRET", insecureDefaultSecret),
		Mode:                               env("KOALABYE_MODE", "selfhost"),
		RegistrationEnabled:                envBool("KOALABYE_REGISTRATION_ENABLED", false),
		InviteOnly:                         envBool("KOALABYE_INVITE_ONLY", true),
		InviteRegistrationEnabled:          envBool("KOALABYE_INVITE_REGISTRATION_ENABLED", true),
		SecureCookies:                      envBool("KOALABYE_SECURE_COOKIES", true),
		InstanceName:                       env("KOALABYE_INSTANCE_NAME", "KoalaBye"),
		InstanceSourceURL:                  strings.TrimSpace(os.Getenv("KOALABYE_INSTANCE_SOURCE_URL")),
		BootstrapUsername:                  strings.TrimSpace(os.Getenv("KOALABYE_BOOTSTRAP_ADMIN_USERNAME")),
		BootstrapPassword:                  os.Getenv("KOALABYE_BOOTSTRAP_ADMIN_PASSWORD"),
		BootstrapDisplayName:               strings.TrimSpace(os.Getenv("KOALABYE_BOOTSTRAP_ADMIN_DISPLAY_NAME")),
		DefaultMaxOrganizationsPerUser:     envInt("KOALABYE_DEFAULT_MAX_ORGANIZATIONS_PER_USER", 1),
		DefaultMaxCampaignsPerOrg:          envInt("KOALABYE_DEFAULT_MAX_CAMPAIGNS_PER_ORG", 3),
		DefaultMaxMembersPerOrg:            envInt("KOALABYE_DEFAULT_MAX_MEMBERS_PER_ORG", 5),
		DefaultMaxActiveInvitesPerOrg:      envInt("KOALABYE_DEFAULT_MAX_ACTIVE_INVITES_PER_ORG", 10),
		DefaultMaxMonthlyVisitsPerOrg:      envInt("KOALABYE_DEFAULT_MAX_MONTHLY_VISITS_PER_ORG", 10000),
		DefaultMaxMonthlySubmissionsPerOrg: envInt("KOALABYE_DEFAULT_MAX_MONTHLY_SUBMISSIONS_PER_ORG", 1000),
	}

	if cfg.Mode != "selfhost" && cfg.Mode != "cloud" {
		return Config{}, fmt.Errorf("KOALABYE_MODE must be selfhost or cloud, got %q", cfg.Mode)
	}
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return Config{}, fmt.Errorf("invalid KOALABYE_BASE_URL: %w", err)
	}
	if cfg.InstanceSourceURL != "" && !isSafePublicURL(cfg.InstanceSourceURL) {
		return Config{}, errors.New("KOALABYE_INSTANCE_SOURCE_URL must use https, except localhost development URLs")
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
	if cfg.DefaultMaxOrganizationsPerUser < 1 || cfg.DefaultMaxCampaignsPerOrg < 0 ||
		cfg.DefaultMaxMembersPerOrg < 1 || cfg.DefaultMaxActiveInvitesPerOrg < 0 ||
		cfg.DefaultMaxMonthlyVisitsPerOrg < 0 || cfg.DefaultMaxMonthlySubmissionsPerOrg < 0 {
		return Config{}, errors.New("default safety limits must be non-negative and owner/member limits at least 1")
	}
	return cfg, nil
}

func isSafePublicURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		slog.Warn("invalid integer environment value; using fallback", "key", key)
		return fallback
	}
	return value
}

func (c Config) isLocalDevelopment() bool {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
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
