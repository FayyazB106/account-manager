-- Consolidated SQLite schema (equivalent to Postgres migrations 0001–0018).
-- Applied once on a fresh database; tracked via PRAGMA user_version.
--
-- Conventions for the SQLite port:
--   * IDs are opaque 32-char hex strings (DEFAULT lower(hex(randomblob(16)))),
--     replacing Postgres UUID / gen_random_uuid().
--   * BYTEA -> BLOB, TIMESTAMPTZ -> TIMESTAMP, DATE stays DATE (modernc parses
--     TIMESTAMP/DATE columns into time.Time), BOOLEAN -> stored as 0/1,
--     DOUBLE PRECISION -> REAL, now() -> CURRENT_TIMESTAMP.

-- category: first-class categories (name + react-icons icon), manually ordered.
CREATE TABLE category (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name       TEXT NOT NULL UNIQUE,
    name_ar    TEXT,
    icon       TEXT NOT NULL DEFAULT 'FaFolder',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- account: one stored credential / subscription. The password lives only as the
-- two envelope-encrypted blobs (wrapped DEK + ciphertext); no plaintext column.
CREATE TABLE account (
    id                    TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    category_id           TEXT REFERENCES category (id) ON DELETE SET NULL,
    service               TEXT NOT NULL,
    website               TEXT,
    login                 TEXT,
    password_wrapped_dek  BLOB NOT NULL,
    password_ciphertext   BLOB NOT NULL,
    next_payment_date     DATE,
    notes                 TEXT,
    billing_period_months INTEGER NOT NULL DEFAULT 0,
    amount                REAL,
    currency              TEXT NOT NULL DEFAULT 'USD', -- the currency `amount` is in (each account has its own)
    rotate_every_days     INTEGER,
    status                TEXT NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active', 'inactive')),
    totp_secret           BLOB,                       -- encrypted 2FA secret for this service (NULL = none)
    breach_count          INTEGER,                     -- times this password appears in HIBP (NULL = never checked)
    breach_checked_at     TIMESTAMP,                   -- when breach monitoring last checked it
    password_changed_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_account_category_id ON account (category_id);
CREATE INDEX idx_account_next_payment ON account (next_payment_date);
CREATE INDEX idx_account_status ON account (status);

-- password_history: previous (envelope-encrypted) passwords per account, newest
-- kept up to a cap; populated when a password changes.
CREATE TABLE password_history (
    id                   TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    account_id           TEXT NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    password_wrapped_dek BLOB NOT NULL,
    password_ciphertext  BLOB NOT NULL,
    changed_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_pwhist_account ON password_history (account_id);

-- account_user: one SEAT per row (who uses the account); position keeps order.
CREATE TABLE account_user (
    id            TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    account_id    TEXT NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    user_name     TEXT NOT NULL,
    user_name_ar  TEXT,
    position      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_account_user_account ON account_user (account_id);

-- attachment: envelope-encrypted invoice files tied to an account.
CREATE TABLE attachment (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    account_id   TEXT NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    filename     TEXT NOT NULL,
    mime_type    TEXT,
    wrapped_dek  BLOB NOT NULL,
    ciphertext   BLOB NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_attachment_account ON attachment (account_id);

-- payment: a recorded payment EVENT for an account — the actual ledger, distinct
-- from the account's forward-looking next_payment_date. Auto-logged when a
-- subscription is renewed ("Mark as paid"), or added/edited manually. `amount` is
-- expressed in `currency`. `attachment_id` optionally links the invoice file and
-- is cleared (not cascaded) if that attachment is later deleted, so the payment
-- record survives. `source` distinguishes auto-logged from manual rows.
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

-- admin: the trusted tech-support members who log in.
CREATE TABLE admin (
    id                 TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    email              TEXT NOT NULL UNIQUE,
    name               TEXT NOT NULL DEFAULT '',
    name_ar            TEXT,
    auth_hash          TEXT NOT NULL,
    kek_salt           BLOB NOT NULL,
    wrapped_vault_key  BLOB NOT NULL,
    totp_secret        BLOB,                       -- encrypted TOTP secret (NULL = not set up)
    totp_enabled       INTEGER NOT NULL DEFAULT 0, -- 1 once enrollment is confirmed
    monthly_report     INTEGER NOT NULL DEFAULT 0, -- 1 = email me last month's payment history on the 1st
    monthly_report_last TEXT   NOT NULL DEFAULT '', -- "YYYY-MM" of the last report sent (or the baseline at opt-in)
    payment_reminders  INTEGER NOT NULL DEFAULT 0, -- 1 = email me daily while a payment is due within 7 days or overdue
    payment_reminder_last TEXT NOT NULL DEFAULT '', -- "YYYY-MM-DD" of the last reminder sent (or the baseline at opt-in)
    rotation_reminders INTEGER NOT NULL DEFAULT 0, -- 1 = email me daily while a password is due for rotation within 7 days or overdue
    rotation_reminder_last TEXT NOT NULL DEFAULT '', -- "YYYY-MM-DD" of the last rotation reminder sent (or the baseline at opt-in)
    upcoming_payments  INTEGER NOT NULL DEFAULT 0, -- 1 = email me the month's upcoming payments on the 1st
    upcoming_payments_last TEXT NOT NULL DEFAULT '', -- "YYYY-MM" of the last upcoming-payments email handled (or the baseline at opt-in)
    payment_reminder_days INTEGER NOT NULL DEFAULT 255, -- which days the payment reminder is sent: bit n = n days before due (0..7); 255 = all
    rotation_reminder_days INTEGER NOT NULL DEFAULT 255, -- same, for password-rotation reminders
    breach_alerts      INTEGER NOT NULL DEFAULT 0, -- 1 = email me right away when a scan finds a newly breached password
    language           TEXT    NOT NULL DEFAULT 'en', -- the admin's UI language ("en"/"ar"); emails to them are written in it
    created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- breach_alert_pending: breach alerts that could not be delivered yet (mail server
-- unreachable); retried until sent. Rows vanish with the admin or the account.
CREATE TABLE breach_alert_pending (
    admin_id   TEXT NOT NULL REFERENCES admin (id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    PRIMARY KEY (admin_id, account_id)
);

-- admin_backup_code: single-use recovery codes for MFA (hashed).
CREATE TABLE admin_backup_code (
    id        TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    admin_id  TEXT NOT NULL REFERENCES admin (id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_backup_code_admin ON admin_backup_code (admin_id);

-- person: people who occupy account seats (distinct from admins).
CREATE TABLE person (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name       TEXT NOT NULL,
    name_ar    TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- app_setting: shared key/value settings.
CREATE TABLE app_setting (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- audit_log: append-only record of security-relevant actions (who did what,
-- from where, when). actor_email is a snapshot so entries survive admin deletion.
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

-- currency: activatable currencies; USD is the base (rate 1, always active).
CREATE TABLE currency (
    code       TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    active     BOOLEAN NOT NULL DEFAULT 0,
    rate       REAL NOT NULL DEFAULT 0,
    updated_at TIMESTAMP
);

-- Seed data ------------------------------------------------------------------

INSERT INTO category (name, icon, position) VALUES
    ('Design', 'FaPalette', 0),
    ('3D', 'FaCube', 1),
    ('IT', 'FaServer', 2),
    ('General', 'FaFolder', 3);

INSERT INTO app_setting (key, value) VALUES ('week_start', '0');

INSERT INTO currency (code, name, active, rate) VALUES
    ('USD', 'US Dollar', 1, 1),
    ('EUR', 'Euro', 0, 0),
    ('GBP', 'British Pound', 0, 0),
    ('SAR', 'Saudi Riyal', 0, 0),
    ('AED', 'UAE Dirham', 0, 0),
    ('PKR', 'Pakistani Rupee', 0, 0),
    ('INR', 'Indian Rupee', 0, 0),
    ('JPY', 'Japanese Yen', 0, 0),
    ('CNY', 'Chinese Yuan', 0, 0),
    ('CAD', 'Canadian Dollar', 0, 0),
    ('AUD', 'Australian Dollar', 0, 0),
    ('CHF', 'Swiss Franc', 0, 0),
    ('TRY', 'Turkish Lira', 0, 0),
    ('EGP', 'Egyptian Pound', 0, 0),
    ('ZAR', 'South African Rand', 0, 0),
    ('BRL', 'Brazilian Real', 0, 0),
    ('SGD', 'Singapore Dollar', 0, 0),
    ('MYR', 'Malaysian Ringgit', 0, 0);

-- Refresh tokens: long-lived, httpOnly-cookie sessions. Only a hash of the
-- secret is stored; rotated on use, with `used_at` enabling reuse detection.
CREATE TABLE refresh_token (
    id         TEXT PRIMARY KEY,
    admin_id   TEXT NOT NULL REFERENCES admin (id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    used_at    TIMESTAMP
);
CREATE INDEX idx_refresh_admin ON refresh_token (admin_id);
