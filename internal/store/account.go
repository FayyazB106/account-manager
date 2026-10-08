package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Account is one stored credential. The password is NOT here as plaintext — it
// lives only as the two envelope-encrypted blobs.
type Account struct {
	ID               string
	CategoryID       *string
	Service             string
	Website             *string // optional URL
	Login               *string // optional
	BillingPeriodMonths int
	Amount              *float64   // optional billing amount ($)
	NextPaymentDate     *time.Time // optional
	Notes            *string    // optional
	RotateEveryDays  *int       // optional rotation policy
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// Secret material (envelope encryption). Only loaded when revealing.
	PasswordWrappedDEK []byte
	PasswordCiphertext []byte
}

// AccountListItem is the safe, secret-free view returned by list endpoints.
type AccountListItem struct {
	ID                string     `json:"id"`
	CategoryID        *string    `json:"category_id,omitempty"`
	CategoryName        *string    `json:"category_name,omitempty"`
	CategoryIcon        *string    `json:"category_icon,omitempty"`
	Service             string     `json:"service"`
	Website             *string    `json:"website,omitempty"`
	Login               *string    `json:"login,omitempty"`
	BillingPeriodMonths int        `json:"billing_period_months"`
	Amount              *float64   `json:"amount,omitempty"`
	NextPaymentDate     *time.Time `json:"next_payment_date,omitempty"`
	PasswordChangedAt time.Time  `json:"password_changed_at"`
	RotateEveryDays   *int       `json:"rotate_every_days,omitempty"`
	Status            string     `json:"status"`
	Notes             *string    `json:"notes,omitempty"`
	HasTOTP           bool       `json:"has_totp"` // a 2FA secret is stored for this account
	BreachCount       *int       `json:"breach_count,omitempty"`       // HIBP hits (nil = never checked)
	BreachCheckedAt   *time.Time `json:"breach_checked_at,omitempty"`
	Seats             []string   `json:"seats"`
	SeatsAr           []*string  `json:"seats_ar,omitempty"` // Arabic seat names, aligned by position
}

type AccountStore struct {
	db *sql.DB
}

func NewAccountStore(db *sql.DB) *AccountStore {
	return &AccountStore{db: db}
}

// Create inserts a new account. The caller passes the already-encrypted secret
// blobs — the store never sees or handles plaintext passwords.
func (s *AccountStore) Create(ctx context.Context, a *Account) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO account
		   (category_id, service, website, login, billing_period_months, amount, next_payment_date, notes,
		    rotate_every_days, password_wrapped_dek, password_ciphertext)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		a.CategoryID, a.Service, a.Website, a.Login, a.BillingPeriodMonths, a.Amount, dateParam(a.NextPaymentDate), a.Notes,
		a.RotateEveryDays, a.PasswordWrappedDEK, a.PasswordCiphertext,
	).Scan(&id)
	return id, err
}

// List returns all accounts as secret-free metadata, newest first.
//
// Seats live in account_user (one row per seat). SQLite has no array_agg, so we
// aggregate the ordered names into a JSON array per account and unmarshal it.
func (s *AccountStore) List(ctx context.Context) ([]AccountListItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.category_id, c.name, c.icon, a.service, a.website, a.login,
		        a.billing_period_months, a.amount, a.next_payment_date,
		        a.password_changed_at, a.rotate_every_days, a.status, a.notes,
		        (a.totp_secret IS NOT NULL) AS has_totp, a.breach_count, a.breach_checked_at,
		        (SELECT json_group_array(user_name)
		           FROM (SELECT user_name FROM account_user WHERE account_id = a.id ORDER BY position)) AS seats,
		        (SELECT json_group_array(user_name_ar)
		           FROM (SELECT user_name_ar FROM account_user WHERE account_id = a.id ORDER BY position)) AS seats_ar
		 FROM account a
		 LEFT JOIN category c ON c.id = a.category_id
		 ORDER BY a.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AccountListItem
	for rows.Next() {
		var it AccountListItem
		var seatsJSON, seatsArJSON string
		if err := rows.Scan(&it.ID, &it.CategoryID, &it.CategoryName, &it.CategoryIcon,
			&it.Service, &it.Website, &it.Login,
			&it.BillingPeriodMonths, &it.Amount, &it.NextPaymentDate,
			&it.PasswordChangedAt, &it.RotateEveryDays, &it.Status, &it.Notes,
			&it.HasTOTP, &it.BreachCount, &it.BreachCheckedAt, &seatsJSON, &seatsArJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(seatsJSON), &it.Seats); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(seatsArJSON), &it.SeatsAr); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// UpdateMeta updates the non-secret fields of an account.
func (s *AccountStore) UpdateMeta(ctx context.Context, a *Account) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE account SET
		   category_id = $2, service = $3, website = $4, login = $5, billing_period_months = $6,
		   amount = $7, next_payment_date = $8, notes = $9, rotate_every_days = $10,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		a.ID, a.CategoryID, a.Service, a.Website, a.Login, a.BillingPeriodMonths,
		a.Amount, dateParam(a.NextPaymentDate), a.Notes, a.RotateEveryDays)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// passwordHistoryCap is how many previous passwords we keep per account.
const passwordHistoryCap = 10

// UpdatePassword replaces the encrypted secret and resets password_changed_at,
// so rotation reminders restart from the change. The password being replaced is
// first snapshotted into password_history (capped to the newest N), all in one
// transaction so a failure can't leave a half-updated state.
func (s *AccountStore) UpdatePassword(ctx context.Context, id string, wrappedDEK, ciphertext []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Read the current (about-to-be-replaced) password so we can archive it.
	var oldDEK, oldCipher []byte
	err = tx.QueryRowContext(ctx,
		`SELECT password_wrapped_dek, password_ciphertext FROM account WHERE id = $1`, id).Scan(&oldDEK, &oldCipher)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO password_history (account_id, password_wrapped_dek, password_ciphertext) VALUES ($1, $2, $3)`,
		id, oldDEK, oldCipher); err != nil {
		return err
	}
	// Keep only the newest passwordHistoryCap entries for this account. Tiebreak
	// on rowid (insertion order) since changed_at only has second precision.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM password_history
		 WHERE account_id = $1 AND id NOT IN (
		     SELECT id FROM password_history WHERE account_id = $1
		     ORDER BY changed_at DESC, rowid DESC LIMIT $2)`,
		id, passwordHistoryCap); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE account SET
		   password_wrapped_dek = $2, password_ciphertext = $3,
		   password_changed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP,
		   breach_count = NULL, breach_checked_at = NULL
		 WHERE id = $1`,
		id, wrappedDEK, ciphertext); err != nil {
		return err
	}
	return tx.Commit()
}

// PasswordHistoryMeta is one archived password's id + when it was replaced (no
// secret material — reveal fetches that separately).
type PasswordHistoryMeta struct {
	ID        string    `json:"id"`
	ChangedAt time.Time `json:"changed_at"`
}

// ListHistory returns an account's previous passwords, newest first (metadata only).
func (s *AccountStore) ListHistory(ctx context.Context, accountID string) ([]PasswordHistoryMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, changed_at FROM password_history WHERE account_id = $1 ORDER BY changed_at DESC, rowid DESC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PasswordHistoryMeta
	for rows.Next() {
		var m PasswordHistoryMeta
		if err := rows.Scan(&m.ID, &m.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// HistorySecret is one archived password's encrypted blobs.
type HistorySecret struct {
	WrappedDEK []byte
	Ciphertext []byte
}

// ListHistorySecrets returns all of an account's archived password blobs, OLDEST
// first, in one query (for export — the caller decrypts them).
func (s *AccountStore) ListHistorySecrets(ctx context.Context, accountID string) ([]HistorySecret, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT password_wrapped_dek, password_ciphertext FROM password_history
		 WHERE account_id = $1 ORDER BY changed_at ASC, rowid ASC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistorySecret
	for rows.Next() {
		var h HistorySecret
		if err := rows.Scan(&h.WrappedDEK, &h.Ciphertext); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// GetHistorySecret loads one archived password's encrypted blobs for reveal,
// scoped to the account so an id from another account can't be fetched.
func (s *AccountStore) GetHistorySecret(ctx context.Context, accountID, historyID string) (wrappedDEK, ciphertext []byte, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT password_wrapped_dek, password_ciphertext FROM password_history WHERE id = $1 AND account_id = $2`,
		historyID, accountID,
	).Scan(&wrappedDEK, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	return wrappedDEK, ciphertext, err
}

// SetStatus sets an account active or inactive. Activating a recurring account
// (billing period > 0) renews its next payment date to today + one billing
// period, so a reactivated subscription starts a fresh cycle.
func (s *AccountStore) SetStatus(ctx context.Context, id, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE account SET
		   status = $2,
		   next_payment_date = CASE
		     WHEN $2 = 'active' AND billing_period_months > 0
		     THEN date('now', '+' || billing_period_months || ' months')
		     ELSE next_payment_date
		   END,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrNotInactive means a delete was attempted on an active account.
var ErrNotInactive = errors.New("account must be inactive to delete")

// Delete removes an account, but ONLY if it is inactive. The status guard is
// enforced atomically in the WHERE clause.
func (s *AccountStore) Delete(ctx context.Context, id string) error {
	// Distinguish "not found" from "active" so we can return a useful error.
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM account WHERE id = $1`, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "inactive" {
		return ErrNotInactive
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM account WHERE id = $1 AND status = 'inactive'`, id)
	return err
}

// Extend advances next_payment_date by billing_period_months. Returns the new
// date, or ErrNotFound if the account has no date/period to extend. The matching
// payment is recorded separately via the ledger (the "Mark as paid" form), so
// this only moves the renewal date forward.
func (s *AccountStore) Extend(ctx context.Context, id string) (*time.Time, error) {
	var next time.Time
	err := s.db.QueryRowContext(ctx,
		`UPDATE account
		 SET next_payment_date = date(next_payment_date, '+' || billing_period_months || ' months'),
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1 AND next_payment_date IS NOT NULL AND billing_period_months > 0
		 RETURNING next_payment_date`,
		id,
	).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &next, nil
}

// ReplaceSeats replaces all seats for an account with the given names (one seat
// per entry; a name may be empty). Done in a transaction so a failure can't
// leave a half-updated seat list.
func (s *AccountStore) ReplaceSeats(ctx context.Context, accountID string, names []string, namesAr []*string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM account_user WHERE account_id = $1`, accountID); err != nil {
		return err
	}
	for i, name := range names {
		var nameAr *string
		if i < len(namesAr) {
			nameAr = namesAr[i]
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO account_user (account_id, user_name, user_name_ar, position) VALUES ($1, $2, $3, $4)`,
			accountID, name, nameAr, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSecret loads only the encrypted secret blobs for one account (for reveal).
func (s *AccountStore) GetSecret(ctx context.Context, id string) (wrappedDEK, ciphertext []byte, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT password_wrapped_dek, password_ciphertext FROM account WHERE id = $1`,
		id,
	).Scan(&wrappedDEK, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	return wrappedDEK, ciphertext, err
}

// SetTOTP stores (or replaces) the encrypted 2FA secret for an account.
func (s *AccountStore) SetTOTP(ctx context.Context, id string, encSecret []byte) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE account SET totp_secret = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id, encSecret)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearTOTP removes an account's stored 2FA secret.
func (s *AccountStore) ClearTOTP(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE account SET totp_secret = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrNoTOTP means the account has no stored 2FA secret.
var ErrNoTOTP = errors.New("no totp secret for account")

// AccountTOTPSecret is one account's id + encrypted 2FA secret.
type AccountTOTPSecret struct {
	ID     string
	Secret []byte
}

// AllTOTPSecrets returns every account that has a stored 2FA secret (for the
// authenticator list).
func (s *AccountStore) AllTOTPSecrets(ctx context.Context) ([]AccountTOTPSecret, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, totp_secret FROM account WHERE totp_secret IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountTOTPSecret
	for rows.Next() {
		var a AccountTOTPSecret
		if err := rows.Scan(&a.ID, &a.Secret); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetTOTP returns an account's encrypted 2FA secret, or ErrNoTOTP if none is set.
func (s *AccountStore) GetTOTP(ctx context.Context, id string) ([]byte, error) {
	var enc []byte
	err := s.db.QueryRowContext(ctx, `SELECT totp_secret FROM account WHERE id = $1`, id).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, ErrNoTOTP
	}
	return enc, nil
}

// AccountSecret is one account's id + encrypted password blobs (for the breach
// monitor to decrypt and check).
type AccountSecret struct {
	ID                 string
	PasswordWrappedDEK []byte
	PasswordCiphertext []byte
}

// ListActiveSecrets returns the encrypted passwords of all active accounts, for
// breach monitoring.
func (s *AccountStore) ListActiveSecrets(ctx context.Context) ([]AccountSecret, error) {
	return s.listSecrets(ctx, "WHERE status = 'active'")
}

// AllSecrets returns every account's encrypted password (for the Vault Health scan).
func (s *AccountStore) AllSecrets(ctx context.Context) ([]AccountSecret, error) {
	return s.listSecrets(ctx, "")
}

func (s *AccountStore) listSecrets(ctx context.Context, where string) ([]AccountSecret, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, password_wrapped_dek, password_ciphertext FROM account `+where)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountSecret
	for rows.Next() {
		var a AccountSecret
		if err := rows.Scan(&a.ID, &a.PasswordWrappedDEK, &a.PasswordCiphertext); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetBreach records an account's HIBP hit count and the check time.
func (s *AccountStore) SetBreach(ctx context.Context, id string, count int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE account SET breach_count = $2, breach_checked_at = CURRENT_TIMESTAMP WHERE id = $1`, id, count)
	return err
}

// LastBreachCheck returns the most recent breach-check time across all accounts
// (zero time if never run).
func (s *AccountStore) LastBreachCheck(ctx context.Context) (time.Time, error) {
	var t sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(breach_checked_at) FROM account`).Scan(&t); err != nil {
		return time.Time{}, err
	}
	return parseSQLiteTS(t.String), nil
}

// BreachSummary returns the last check time and how many accounts are breached.
func (s *AccountStore) BreachSummary(ctx context.Context) (last time.Time, breached int, err error) {
	var t sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT MAX(breach_checked_at), COALESCE(SUM(breach_count > 0), 0) FROM account`).Scan(&t, &breached)
	return parseSQLiteTS(t.String), breached, err
}
