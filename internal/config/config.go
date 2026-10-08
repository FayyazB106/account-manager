// Package config centralizes all runtime configuration. Everything the app needs
// (data directory, secrets, port) is read from the environment in ONE place, so
// no secrets or paths are ever hardcoded across the codebase.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	DataDir   string // base directory for the DB, key file, and backups
	DBPath    string // SQLite database file
	KeyPath   string // passphrase-sealed master key file
	JWTSecret string // signing secret; empty means "load or create one in DataDir"
	Port      string
	BackupDir string // directory for scheduled DB backups
	TLSCert   string // path to a TLS certificate (PEM); enables HTTPS when set with TLSKey
	TLSKey    string // path to the matching TLS private key (PEM)
}

// Load reads configuration from the environment. In development it first loads a
// .env file (ignored if absent, e.g. in production where real env vars are set).
//
// The app is a single self-contained binary backed by an embedded SQLite file;
// there is no external database to configure. Secrets are resolved at startup:
// the master key is either taken from MASTER_KEY (unattended override) or
// unsealed from KeyPath with a passphrase; the JWT secret is taken from
// JWT_SECRET or auto-created under DataDir. So a personal install needs no env
// vars at all beyond the passphrase prompt.
func Load() (*Config, error) {
	// Best-effort: fine if .env doesn't exist (production sets real env vars).
	_ = godotenv.Load()

	dataDir := getenvDefault("DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	return &Config{
		DataDir:   dataDir,
		DBPath:    getenvDefault("DB_PATH", filepath.Join(dataDir, "account-manager.db")),
		KeyPath:   getenvDefault("KEY_PATH", filepath.Join(dataDir, "master.key")),
		JWTSecret: os.Getenv("JWT_SECRET"),
		Port:      getenvDefault("PORT", "8080"),
		BackupDir: getenvDefault("BACKUP_DIR", filepath.Join(dataDir, "backups")),
		TLSCert:   os.Getenv("TLS_CERT"),
		TLSKey:    os.Getenv("TLS_KEY"),
	}, nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
