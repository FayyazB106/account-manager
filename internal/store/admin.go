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
	ID                string
	Email             string
	Name              string
	NameAr            *string // optional Arabic display name
	AuthHash          string  // PHC-encoded Argon2id hash (for login)
	KEKSalt           []byte  // salt to re-derive the key-encryption-key
	WrappedVaultKey   []byte  // shared Vault Key, encrypted with this admin's KEK
	TOTPSecret        []byte  // encrypted TOTP secret (nil = not set up)
	TOTPEnabled       bool    // MFA is active for this admin
	MonthlyReport     bool    // email last month's payment history on the 1st
	PaymentReminders  bool    // email daily while a payment is due within 7 days or overdue
	RotationReminders bool    // email daily while a password is due for rotation within 7 days or overdue
	UpcomingPayments  bool    // email the month's upcoming payments on the 1st
	BreachAlerts      bool    // email right away when a scan finds a newly breached password
	Language          string  // UI language ("en"/"ar"); emails to this admin are written in it
	CreatedAt         time.Time
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
		`SELECT id, email, name, name_ar, auth_hash, kek_salt, wrapped_vault_key, totp_secret, totp_enabled, monthly_report, payment_reminders, rotation_reminders, upcoming_payments, breach_alerts, language, created_at
		 FROM admin WHERE email = $1`,
		email,
	).Scan(&a.ID, &a.Email, &a.Name, &a.NameAr, &a.AuthHash, &a.KEKSalt, &a.WrappedVaultKey, &a.TOTPSecret, &a.TOTPEnabled, &a.MonthlyReport, &a.PaymentReminders, &a.RotationReminders, &a.UpcomingPayments, &a.BreachAlerts, &a.Language, &a.CreatedAt)
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
		`SELECT id, email, name, name_ar, auth_hash, kek_salt, wrapped_vault_key, totp_secret, totp_enabled, monthly_report, payment_reminders, rotation_reminders, upcoming_payments, breach_alerts, language, created_at
		 FROM admin WHERE id = $1`,
		id,
	).Scan(&a.ID, &a.Email, &a.Name, &a.NameAr, &a.AuthHash, &a.KEKSalt, &a.WrappedVaultKey, &a.TOTPSecret, &a.TOTPEnabled, &a.MonthlyReport, &a.PaymentReminders, &a.RotationReminders, &a.UpcomingPayments, &a.BreachAlerts, &a.Language, &a.CreatedAt)
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
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	Name        string  `json:"name"`
	NameAr      *string `json:"name_ar,omitempty"`
	TOTPEnabled bool    `json:"totp_enabled"`
	// MonthlyReport is whether this admin gets last month's payment history by email.
	MonthlyReport bool `json:"monthly_report"`
	// PaymentReminders is whether this admin gets daily due-payment reminders.
	PaymentReminders bool `json:"payment_reminders"`
	// RotationReminders is whether this admin gets daily password-rotation reminders.
	RotationReminders bool `json:"rotation_reminders"`
	// UpcomingPayments is whether this admin gets the monthly upcoming-payments email.
	UpcomingPayments bool `json:"upcoming_payments"`
	// PaymentReminderDays / RotationReminderDays are the days each reminder is sent:
	// 7..0 = days before the due date. Only set by GET /profile.
	PaymentReminderDays  []int `json:"payment_reminder_days,omitempty"`
	RotationReminderDays []int `json:"rotation_reminder_days,omitempty"`
	// BreachAlerts is whether this admin is emailed when a scan finds a new breach.
	BreachAlerts bool `json:"breach_alerts"`
	// Language is the admin's UI language, which their emails follow.
	Language  string    `json:"language"`
	CreatedAt time.Time `json:"created_at"`
}

// List returns all admins as secret-free info, oldest first.
func (s *AdminStore) List(ctx context.Context) ([]AdminInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, email, name, name_ar, totp_enabled, monthly_report, payment_reminders, rotation_reminders, upcoming_payments, breach_alerts, language, created_at FROM admin ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminInfo
	for rows.Next() {
		var a AdminInfo
		if err := rows.Scan(&a.ID, &a.Email, &a.Name, &a.NameAr, &a.TOTPEnabled, &a.MonthlyReport, &a.PaymentReminders, &a.RotationReminders, &a.UpcomingPayments, &a.BreachAlerts, &a.Language, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Monthly payment report ------------------------------------------------

// SetLanguage records the admin's UI language so their emails follow it. Only
// "en" and "ar" are accepted.
func (s *AdminStore) SetLanguage(ctx context.Context, id, lang string) error {
	if lang != "en" && lang != "ar" {
		return errors.New("unsupported language")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET language = $2 WHERE id = $1`, id, lang)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMonthlyReport turns the monthly payment-history email on or off for an admin.
// When enabling, `baseline` ("YYYY-MM", the month that just ended) is recorded as
// already-sent so opting in mid-month does not trigger an immediate backfill — the
// first report arrives when the *next* month completes.
func (s *AdminStore) SetMonthlyReport(ctx context.Context, id string, on bool, baseline string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET monthly_report = $2,
		   monthly_report_last = CASE WHEN $2 = 1 THEN $3 ELSE monthly_report_last END,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		id, on, baseline)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPaymentReminders turns the daily due-payment reminder on or off for an admin.
// When enabling, `baseline` ("YYYY-MM-DD", today) is recorded as already-sent so
// opting in doesn't fire a reminder the same day — the first one is tomorrow's.
func (s *AdminStore) SetPaymentReminders(ctx context.Context, id string, on bool, baseline string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET payment_reminders = $2,
		   payment_reminder_last = CASE WHEN $2 = 1 THEN $3 ELSE payment_reminder_last END,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		id, on, baseline)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ListReminderDue returns opted-in admins who have not yet been sent a reminder
// on `day` ("YYYY-MM-DD") — i.e. at most one reminder per admin per day.
func (s *AdminStore) ListReminderDue(ctx context.Context, day string) ([]ReportRecipient, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, COALESCE(name_ar, ''), language, payment_reminder_days FROM admin WHERE payment_reminders = 1 AND payment_reminder_last < $1 ORDER BY created_at`,
		day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRecipient
	for rows.Next() {
		var r ReportRecipient
		if err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.NameAr, &r.Language, &r.Days); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkReminderSent records that `day`'s reminder was delivered to this admin.
func (s *AdminStore) MarkReminderSent(ctx context.Context, id, day string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin SET payment_reminder_last = $2 WHERE id = $1 AND payment_reminder_last < $2`, id, day)
	return err
}

// SetRotationReminders turns the daily password-rotation reminder on or off for an
// admin. When enabling, `baseline` ("YYYY-MM-DD", today) is recorded as already-sent
// so opting in doesn't fire a reminder the same day.
func (s *AdminStore) SetRotationReminders(ctx context.Context, id string, on bool, baseline string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET rotation_reminders = $2,
		   rotation_reminder_last = CASE WHEN $2 = 1 THEN $3 ELSE rotation_reminder_last END,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		id, on, baseline)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRotationDue returns opted-in admins who have not yet been sent a rotation
// reminder on `day` ("YYYY-MM-DD") — at most one per admin per day.
func (s *AdminStore) ListRotationDue(ctx context.Context, day string) ([]ReportRecipient, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, COALESCE(name_ar, ''), language, rotation_reminder_days FROM admin WHERE rotation_reminders = 1 AND rotation_reminder_last < $1 ORDER BY created_at`,
		day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRecipient
	for rows.Next() {
		var r ReportRecipient
		if err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.NameAr, &r.Language, &r.Days); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkRotationSent records that `day`'s rotation reminder was delivered to this admin.
func (s *AdminStore) MarkRotationSent(ctx context.Context, id, day string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin SET rotation_reminder_last = $2 WHERE id = $1 AND rotation_reminder_last < $2`, id, day)
	return err
}

// SetUpcomingPayments turns the monthly upcoming-payments email on or off for an
// admin. When enabling, `baseline` ("YYYY-MM", the current month) is recorded as
// already handled so opting in mid-month doesn't send this month's list.
func (s *AdminStore) SetUpcomingPayments(ctx context.Context, id string, on bool, baseline string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET upcoming_payments = $2,
		   upcoming_payments_last = CASE WHEN $2 = 1 THEN $3 ELSE upcoming_payments_last END,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		id, on, baseline)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUpcomingDue returns opted-in admins whose upcoming-payments email for `month`
// ("YYYY-MM") has not been handled yet — at most one per admin per month.
func (s *AdminStore) ListUpcomingDue(ctx context.Context, month string) ([]ReportRecipient, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, COALESCE(name_ar, ''), language FROM admin WHERE upcoming_payments = 1 AND upcoming_payments_last < $1 ORDER BY created_at`,
		month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRecipient
	for rows.Next() {
		var r ReportRecipient
		if err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.NameAr, &r.Language); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkUpcomingSent records that `month`'s upcoming-payments email was handled for
// this admin (delivered, or deliberately skipped because nothing was scheduled).
func (s *AdminStore) MarkUpcomingSent(ctx context.Context, id, month string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin SET upcoming_payments_last = $2 WHERE id = $1 AND upcoming_payments_last < $2`, id, month)
	return err
}

// ReportRecipient is an admin who should receive the monthly report.
type ReportRecipient struct {
	ID       string
	Email    string
	Name     string
	NameAr   string // empty when the admin has no Arabic name
	Language string // "en" or "ar"
	// Days is the admin's chosen reminder days (see DaysToMask); only filled in by the
	// payment and rotation reminder lists.
	Days int
}

// ListReportDue returns opted-in admins whose last-sent month is earlier than
// `month` ("YYYY-MM"), i.e. who have not yet received that month's report.
func (s *AdminStore) ListReportDue(ctx context.Context, month string) ([]ReportRecipient, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, COALESCE(name_ar, ''), language FROM admin WHERE monthly_report = 1 AND monthly_report_last < $1 ORDER BY created_at`,
		month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRecipient
	for rows.Next() {
		var r ReportRecipient
		if err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.NameAr, &r.Language); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkReportSent records that `month`'s report was delivered to this admin.
func (s *AdminStore) MarkReportSent(ctx context.Context, id, month string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin SET monthly_report_last = $2 WHERE id = $1 AND monthly_report_last < $2`, id, month)
	return err
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

// SetBreachAlerts turns the "email me when a new breach is found" alert on or off.
// There is no schedule or baseline: only breaches found after this point are sent.
func (s *AdminStore) SetBreachAlerts(ctx context.Context, id string, on bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET breach_alerts = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id, on)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	if !on {
		return s.ClearBreachPending(ctx, id) // an alert that was still waiting is dropped
	}
	return nil
}

// AddBreachPending queues undelivered breach alerts for an admin (one row per account).
func (s *AdminStore) AddBreachPending(ctx context.Context, adminID string, accountIDs []string) error {
	for _, id := range accountIDs {
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO breach_alert_pending (admin_id, account_id) VALUES ($1, $2)`, adminID, id); err != nil {
			return err
		}
	}
	return nil
}

// ClearBreachPending removes everything queued for an admin (after delivery).
func (s *AdminStore) ClearBreachPending(ctx context.Context, adminID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM breach_alert_pending WHERE admin_id = $1`, adminID)
	return err
}

// ListBreachPendingRecipients returns opted-in admins who still have undelivered
// breach alerts.
func (s *AdminStore) ListBreachPendingRecipients(ctx context.Context) ([]ReportRecipient, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, COALESCE(name_ar, ''), language FROM admin
		 WHERE breach_alerts = 1 AND id IN (SELECT admin_id FROM breach_alert_pending) ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRecipient
	for rows.Next() {
		var r ReportRecipient
		if err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.NameAr, &r.Language); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PendingBreachHits returns the queued alerts for an admin that are still true: the
// account is active and its password is still flagged (with its current count).
func (s *AdminStore) PendingBreachHits(ctx context.Context, adminID string) ([]BreachHit, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.service, COALESCE(a.login, ''), a.breach_count FROM breach_alert_pending p
		 JOIN account a ON a.id = p.account_id
		 WHERE p.admin_id = $1 AND a.status = 'active' AND a.breach_count > 0
		 ORDER BY a.breach_count DESC, a.service`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BreachHit
	for rows.Next() {
		var h BreachHit
		if err := rows.Scan(&h.ID, &h.Service, &h.Login, &h.Count); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ListBreachAlertRecipients returns every admin who has breach alerts turned on.
func (s *AdminStore) ListBreachAlertRecipients(ctx context.Context) ([]ReportRecipient, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, COALESCE(name_ar, ''), language FROM admin WHERE breach_alerts = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRecipient
	for rows.Next() {
		var r ReportRecipient
		if err := rows.Scan(&r.ID, &r.Email, &r.Name, &r.NameAr, &r.Language); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Reminder days are stored as a bit mask: bit n (0..7) = "n days before the due date".
// 255 (all eight bits) is the default. Overdue items are not reminded about; the one
// exception is "Send Now", which uses PreviewDays to list everything flagged.
const (
	MaxReminderDay  = 7
	AllReminderDays = 0xFF
	overdueBit      = 1 << 8
	// PreviewDays is the mask "Send Now" uses: every day in the window plus overdue.
	PreviewDays = AllReminderDays | overdueBit
	// OverdueBit is cleared from stored masks when sending scheduled reminders.
	OverdueBit = overdueBit
)

// DaysToMask turns a list of days (0..7 days before the due date) into the stored mask.
// Duplicates are fine; anything out of range, or an empty list, is an error.
func DaysToMask(days []int) (int, error) {
	mask := 0
	for _, d := range days {
		if d < 0 || d > MaxReminderDay {
			return 0, errors.New("invalid reminder day")
		}
		mask |= 1 << d
	}
	if mask == 0 {
		return 0, errors.New("pick at least one reminder day")
	}
	return mask, nil
}

// MaskToDays is the reverse: 7, 6, ... 0 (days before the due date).
func MaskToDays(mask int) []int {
	out := []int{}
	for d := MaxReminderDay; d >= 0; d-- {
		if mask&(1<<d) != 0 {
			out = append(out, d)
		}
	}
	return out
}

// DayWanted reports whether a reminder should be sent for an item that is `days` days
// from its due date (negative = overdue) given the admin's mask.
func DayWanted(mask, days int) bool {
	if days < 0 {
		return mask&overdueBit != 0
	}
	if days > MaxReminderDay {
		return false
	}
	return mask&(1<<days) != 0
}

// ReminderDays returns an admin's chosen payment and rotation reminder masks.
func (s *AdminStore) ReminderDays(ctx context.Context, id string) (payment, rotation int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT payment_reminder_days, rotation_reminder_days FROM admin WHERE id = $1`, id).Scan(&payment, &rotation)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

// SetPaymentReminderDays / SetRotationReminderDays store an admin's chosen days.
func (s *AdminStore) SetPaymentReminderDays(ctx context.Context, id string, mask int) error {
	return s.setDays(ctx, "payment_reminder_days", id, mask)
}
func (s *AdminStore) SetRotationReminderDays(ctx context.Context, id string, mask int) error {
	return s.setDays(ctx, "rotation_reminder_days", id, mask)
}

func (s *AdminStore) setDays(ctx context.Context, column, id string, mask int) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin SET `+column+` = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id, mask)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}
