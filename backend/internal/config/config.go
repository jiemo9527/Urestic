package config

import (
	"os"
	"strconv"
)

type Config struct {
	Addr              string
	DataDir           string
	DatabasePath      string
	WebDir            string
	DefaultLang       string
	AdminUsername     string
	AdminPassword     string
	AdminPasswordHash string
	AuthEnabled       bool
	SessionTTLHours   int
	RcloneConfig      string
	RcloneImportPath  string
	RcloneCacheDir    string
}

func Load() Config {
	dataDir := env("URESTIC_DATA_DIR", "/app/data")

	return Config{
		Addr:              env("URESTIC_ADDR", ":8085"),
		DataDir:           dataDir,
		DatabasePath:      env("URESTIC_DATABASE_PATH", dataDir+"/urestic.db"),
		WebDir:            env("URESTIC_WEB_DIR", ""),
		DefaultLang:       env("URESTIC_LANG", "zh-CN"),
		AdminUsername:     env("URESTIC_ADMIN_USERNAME", "admin"),
		AdminPassword:     env("URESTIC_ADMIN_PASSWORD", ""),
		AdminPasswordHash: env("URESTIC_ADMIN_PASSWORD_HASH", ""),
		AuthEnabled:       envBool("URESTIC_AUTH_ENABLED", true),
		SessionTTLHours:   envInt("URESTIC_SESSION_TTL_HOURS", 12),
		RcloneConfig:      env("URESTIC_RCLONE_CONFIG", dataDir+"/rclone/rclone.conf"),
		RcloneImportPath:  env("URESTIC_RCLONE_IMPORT_PATH", "/host-rclone/rclone.conf"),
		RcloneCacheDir:    env("URESTIC_RCLONE_CACHE_DIR", dataDir+"/rclone/cache"),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
