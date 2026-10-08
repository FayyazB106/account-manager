// Package store holds database access logic — the SQL queries — kept separate
// from HTTP handlers so the two concerns don't tangle.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// Admin mirrors a row of the admin table.
type Admin struct {
	ID              string
	Email           string
	Name            string
	NameAr          *string // optional Arabic display name
	AuthHash        string  // PHC-encoded Argon2id hash (for login)
	KEKSalt         []byte  // salt to re-derive the key-encryption-key
	WrappedVaultKey []byte  // shared Vault Key, encrypted with this admin's KEK
	TOTPSecret      []byte  // encrypted TOTP secret (nil = not set up)
	TOTPEnabled     bool    // MFA is active for this admin
	CreatedAt       time.Time
}

type AdminStore struct {
	db *sql.DB
}

func NewAdminStore(db *sql.DB) *AdminStore {
	return &AdminStore{db: db}
}

// Count returns how many admins exist. Used to gate bootstrap registration.
func (s *AdminStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM admin`).Scan(&n)
	return n, err
}

// Create inserts a new admin and returns the generated ID.
func (s *AdminStore) Create(ctx context.Context, email, name string, nameAr *string, authHash string, kekSalt, wrappedVaultKey []byte) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO admin (email, name, name_ar, auth_hash, kek_salt, wrapped_vault_key)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		email, name, nameAr, authHash, kekSalt, wrappedVaultKey,
	).Scan(&id)
	return id, err
}

// ErrRegistrationClosed means CreateFirst was called but an admin already exists.
var ErrRegistrationClosed = errors.New("registration is closed")

// CreateFirst inserts the bootstrap admin only if no admin exists yet — the
// insert and the existence check are one atomic statement, so two concurrent
// bootstrap registrations can't both win. Returns ErrRegistrationClosed otherwise.
func (s *AdminStore) CreateFirst(ctx context.Context, email, name string, nameAr *string, authHash string, kekSalt, wrappedVaultKey []byte) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO admin (email, name, name_ar, auth_hash, kek_salt, wrapped_vault_key)
		 SELECT $1, $2, $3, $4, $5, $6
		 WHERE NOT EXISTS (SELECT 1 FROM admin)
		 RETURNING id`,
		email, name, nameAr, authHash, kekSalt, wrappedVaultKey,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrRegistrationClosed
	}
	return id, err
}

// GetByEmail looks up an admin by email, returning ErrNotFound if none.
func (s *AdminStore) GetByEmail(ctx context.Context, email string) (*Admin, error) {
	a := &Admin{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, name_ar, auth_hash, kek_salt, wrapped_vault_key, totp_secret, totp_enabled, created_at
		 FROM admin WHERE email = $1`,
		email,
	).Scan(&a.ID, &a.Email, &a.Name, &a.NameAr, &a.AuthHash, &a.KEKSalt, &a.WrappedVaultKey, &a.TOTPSecret, &a.TOTPEnabled, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetByID looks up an admin by id.
func (s *AdminStore) GetByID(ctx context.Context, id string) (*Admin, error) {
	a := &Admin{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, name_ar, auth_hash, kek_salt, wrapped_vault_key, totp_secret, totp_enabled, created_at
		 FROM admin WHERE id = $1`,
		id,
	).Scan(&a.ID, &a.Email, &a.Name, &a.NameAr, &a.AuthHash, &a.KEKSalt, &a.WrappedVaultKey, &a.TOTPSecret, &a.TOTPEnabled, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// AdminInfo is the safe, secret-free view of an admin for listing.
type AdminInfo struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	NameAr      *string   `json:"name_ar,omitempty"`
	TOTPEnabled bool      `json:"totp_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// List returns all admins as secret-free info, oldest first.
func (s *AdminStore) List(ctx context.Context) ([]AdminInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, name, name_ar, totp_enabled, created_at FROM admin ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminInfo
	for rows.Next() {
		var a AdminInfo
		if err := rows.Scan(&a.ID, &a.Email, &a.Name, &a.NameAr, &a.TOTPEnabled, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateCredentials replaces an admin's login hash, KEK salt, and wrapped Vault
// Key — used when the admin changes their master password.
func (s *AdminStore) UpdateCredentials(ctx context.Context, id, authHash string, kekSalt, wrappedVaultKey []byte) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET auth_hash = $2, kek_salt = $3, wrapped_vault_key = $4, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		id, authHash, kekSalt, wrappedVaultKey)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateProfile updates an admin's display name and email.
func (s *AdminStore) UpdateProfile(ctx context.Context, id, name string, nameAr *string, email string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET name = $2, name_ar = $3, email = $4, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		id, name, nameAr, email)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrLastAdmin means a delete was refused because it would remove the only
// remaining admin (which would lock everyone out).
var ErrLastAdmin = errors.New("cannot delete the last admin")

// Delete removes an admin, refusing to delete the last one. The count guard is
// part of the DELETE statement so it's atomic — two admins can't race each other
// down to zero.
func (s *AdminStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM admin WHERE id = $1 AND (SELECT count(*) FROM admin) > 1`, id)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		// Nothing deleted: either the admin doesn't exist, or it was the last one.
		var exists int
		if e := s.db.QueryRowContext(ctx, `SELECT count(*) FROM admin WHERE id = $1`, id).Scan(&exists); e != nil {
			return e
		}
		if exists > 0 {
			return ErrLastAdmin
		}
		return ErrNotFound
	}
	return nil
}

// --- TOTP MFA ---------------------------------------------------------------

// SetTOTPSecret stores a new (encrypted) TOTP secret for enrollment, leaving MFA
// disabled until the admin confirms a code via EnableTOTP.
func (s *AdminStore) SetTOTPSecret(ctx context.Context, id string, encSecret []byte) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET totp_secret = $2, totp_enabled = 0, updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
		id, encSecret)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// EnableTOTP marks MFA active after a code has been verified.
func (s *AdminStore) EnableTOTP(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET totp_enabled = 1, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// DisableTOTP turns MFA off and clears the secret and any backup codes.
func (s *AdminStore) DisableTOTP(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE admin SET totp_secret = NULL, totp_enabled = 0, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_backup_code WHERE admin_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplaceBackupCodes stores a fresh set of hashed backup codes, replacing any
// existing ones.
func (s *AdminStore) ReplaceBackupCodes(ctx context.Context, id string, hashes []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_backup_code WHERE admin_id = $1`, id); err != nil {
		return err
	}
	for _, h := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admin_backup_code (admin_id, code_hash) VALUES ($1, $2)`, id, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UseBackupCode consumes an unused backup code (marks it used) and reports
// whether it matched.
func (s *AdminStore) UseBackupCode(ctx context.Context, id, hash string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin_backup_code SET used = 1 WHERE admin_id = $1 AND code_hash = $2 AND used = 0`,
		id, hash)
	if err != nil {
		return false, err
	}
	return affected(res) > 0, nil
}

// BackupCodesRemaining counts an admin's unused backup codes.
func (s *AdminStore) BackupCodesRemaining(ctx context.Context, id string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM admin_backup_code WHERE admin_id = $1 AND used = 0`, id).Scan(&n)
	return n, err
}
