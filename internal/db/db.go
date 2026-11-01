// Package db manages the embedded SQLite database: opening it, applying the
// schema, and handing back a *sql.DB the stores use.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
)

//go:embed schema.sql
var schemaSQL string

// schemaVersion is the latest schema version. A fresh database applies schema.sql
// (which is the full latest schema) and jumps straight to this version; an
// existing database runs each newer step in `migrations`.
const schemaVersion = 17

// migrations holds incremental steps for databases created at an earlier version.
// schema.sql already reflects the latest state, so a fresh install never runs these.
var migrations = map[int]string{
	2: `
ALTER TABLE admin ADD COLUMN totp_secret BLOB;
ALTER TABLE admin ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
CREATE TABLE admin_backup_code (
    id        TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    admin_id  TEXT NOT NULL REFERENCES admin (id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_backup_code_admin ON admin_backup_code (admin_id);
`,
	3: `
CREATE TABLE audit_log (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    actor_email TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    target      TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT '',
    result      TEXT NOT NULL DEFAULT 'ok'
);
CREATE INDEX idx_audit_at ON audit_log (at);
`,
	4: `
CREATE TABLE password_history (
    id                   TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    account_id           TEXT NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    password_wrapped_dek BLOB NOT NULL,
    password_ciphertext  BLOB NOT NULL,
    changed_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_pwhist_account ON password_history (account_id);
`,
	5: `ALTER TABLE account ADD COLUMN totp_secret BLOB;`,
	6: `
ALTER TABLE account ADD COLUMN breach_count INTEGER;
ALTER TABLE account ADD COLUMN breach_checked_at TIMESTAMP;
`,
	7: `
CREATE TABLE refresh_token (
    id         TEXT PRIMARY KEY,
    admin_id   TEXT NOT NULL REFERENCES admin (id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    used_at    TIMESTAMP
);
CREATE INDEX idx_refresh_admin ON refresh_token (admin_id);
`,
	8: `
CREATE TABLE payment (
    id            TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    account_id    TEXT NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    paid_on       DATE NOT NULL,
    amount        REAL NOT NULL,
    currency      TEXT NOT NULL DEFAULT 'USD',
    note          TEXT,
    attachment_id TEXT REFERENCES attachment (id) ON DELETE SET NULL,
    source        TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'auto')),
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_payment_account ON payment (account_id);
CREATE INDEX idx_payment_paid_on ON payment (paid_on);
`,
	9: `
ALTER TABLE admin ADD COLUMN monthly_report INTEGER NOT NULL DEFAULT 0;
ALTER TABLE admin ADD COLUMN monthly_report_last TEXT NOT NULL DEFAULT '';
`,
	10: `
ALTER TABLE admin ADD COLUMN payment_reminders INTEGER NOT NULL DEFAULT 0;
ALTER TABLE admin ADD COLUMN payment_reminder_last TEXT NOT NULL DEFAULT '';
`,
	11: `
ALTER TABLE admin ADD COLUMN rotation_reminders INTEGER NOT NULL DEFAULT 0;
ALTER TABLE admin ADD COLUMN rotation_reminder_last TEXT NOT NULL DEFAULT '';
`,
	12: `
ALTER TABLE admin ADD COLUMN upcoming_payments INTEGER NOT NULL DEFAULT 0;
ALTER TABLE admin ADD COLUMN upcoming_payments_last TEXT NOT NULL DEFAULT '';
`,
	13: `
ALTER TABLE admin ADD COLUMN language TEXT NOT NULL DEFAULT 'en';
`,
	14: `
ALTER TABLE admin ADD COLUMN breach_alerts INTEGER NOT NULL DEFAULT 0;
`,
	15: `
CREATE TABLE IF NOT EXISTS breach_alert_pending (
    admin_id   TEXT NOT NULL REFERENCES admin (id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    PRIMARY KEY (admin_id, account_id)
);
`,
	16: `
ALTER TABLE admin ADD COLUMN payment_reminder_days INTEGER NOT NULL DEFAULT 255;
ALTER TABLE admin ADD COLUMN rotation_reminder_days INTEGER NOT NULL DEFAULT 255;
`,
	17: `
ALTER TABLE account ADD COLUMN currency TEXT NOT NULL DEFAULT 'USD';
`,
}

// Open opens (creating if needed) the SQLite database at path, verifies it, and
// applies the schema on a fresh database.
//
// Pragmas: foreign_keys enforces the ON DELETE rules; WAL improves concurrency;
// busy_timeout waits instead of failing when the single writer is momentarily
// busy. MaxOpenConns(1) serializes writes — simplest way to avoid "database is
// locked" for a small self-hosted app.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	d.SetMaxOpenConns(1)
	if err := d.Ping(); err != nil {
		d.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(d); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// migrate applies the consolidated schema to a fresh database. Versioning uses
// SQLite's built-in PRAGMA user_version so no separate migrations table is
// needed; future schema changes bump the version and add a step here.
func migrate(d *sql.DB) error {
	var v int
	if err := d.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	// Refuse a DB written by a newer build rather than silently lowering its
	// version (which would make this build skip migrations it thinks are applied).
	if v > schemaVersion {
		return fmt.Errorf("database schema is v%d but this build only supports up to v%d — upgrade the app", v, schemaVersion)
	}
	if v == 0 {
		// Fresh database: apply the full current schema and jump to latest, all in
		// one transaction so a crash mid-apply rolls back cleanly (no half-created
		// schema that would re-run and fail with "table already exists").
		return applyStep(d, schemaSQL, schemaVersion)
	}
	// Existing database: apply each newer incremental step in its OWN transaction,
	// stamping user_version inside that same transaction. So each step is atomic
	// and stamped — a failed/interrupted step rolls back and is retried cleanly on
	// the next start (never re-applied on top of a partial success).
	for step := v + 1; step <= schemaVersion; step++ {
		if err := applyStep(d, migrations[step], step); err != nil {
			return fmt.Errorf("apply migration %d: %w", step, err)
		}
	}
	return nil
}

// applyStep runs sqlText (may be empty for a version with no migration) and sets
// user_version to `version`, atomically in one transaction.
func applyStep(d *sql.DB, sqlText string, version int) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if sqlText != "" {
		if _, err := tx.Exec(sqlText); err != nil {
			return err
		}
	}
	// PRAGMA user_version doesn't accept bound parameters; version is an int const.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}
