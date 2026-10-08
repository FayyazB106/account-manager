package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Payment is one recorded payment event for an account. It is the actual ledger,
// separate from the account's forward-looking next_payment_date. `Amount` is in
// `Currency`; `AttachmentID` optionally links an invoice file on the same account.
type Payment struct {
	ID           string    `json:"id"`
	AccountID    string    `json:"account_id"`
	PaidOn       time.Time `json:"paid_on"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	Note         *string   `json:"note,omitempty"`
	AttachmentID *string   `json:"attachment_id,omitempty"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"created_at"`

	// AttachmentName is the linked invoice's filename, joined in for display. It is
	// NULL when no attachment is linked (or the linked one was deleted).
	AttachmentName *string `json:"attachment_name,omitempty"`
}

// PaymentListItem is a payment plus the joined account fields the global Finance
// ledger needs to filter and display across accounts.
type PaymentListItem struct {
	Payment
	Service             string   `json:"service"`
	CategoryID          *string  `json:"category_id,omitempty"`
	CategoryName        *string  `json:"category_name,omitempty"`
	CategoryIcon        *string  `json:"category_icon,omitempty"`
	BillingPeriodMonths int      `json:"billing_period_months"`
	Seats               []string `json:"seats"`
	SeatsAr             []*string `json:"seats_ar,omitempty"`
}

type PaymentStore struct {
	db *sql.DB
}

func NewPaymentStore(db *sql.DB) *PaymentStore {
	return &PaymentStore{db: db}
}

// ListByAccount returns an account's payments, newest first, with the linked
// invoice filename joined in (NULL when none).
func (s *PaymentStore) ListByAccount(ctx context.Context, accountID string) ([]Payment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id, p.account_id, p.paid_on, p.amount, p.currency, p.note,
		        p.attachment_id, p.source, p.created_at, att.filename
		 FROM payment p
		 LEFT JOIN attachment att ON att.id = p.attachment_id
		 WHERE p.account_id = $1
		 ORDER BY p.paid_on DESC, p.created_at DESC, p.rowid DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.AccountID, &p.PaidOn, &p.Amount, &p.Currency,
			&p.Note, &p.AttachmentID, &p.Source, &p.CreatedAt, &p.AttachmentName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListAll returns every payment across all accounts, newest first, joined to the
// account/category and seats so the global ledger can filter and display.
func (s *PaymentStore) ListAll(ctx context.Context) ([]PaymentListItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id, p.account_id, p.paid_on, p.amount, p.currency, p.note,
		        p.attachment_id, p.source, p.created_at, att.filename,
		        a.service, a.category_id, c.name, c.icon, a.billing_period_months,
		        (SELECT json_group_array(user_name)
		           FROM (SELECT user_name FROM account_user WHERE account_id = a.id ORDER BY position)) AS seats,
		        (SELECT json_group_array(user_name_ar)
		           FROM (SELECT user_name_ar FROM account_user WHERE account_id = a.id ORDER BY position)) AS seats_ar
		 FROM payment p
		 JOIN account a ON a.id = p.account_id
		 LEFT JOIN category c ON c.id = a.category_id
		 LEFT JOIN attachment att ON att.id = p.attachment_id
		 ORDER BY p.paid_on DESC, p.created_at DESC, p.rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PaymentListItem
	for rows.Next() {
		var p PaymentListItem
		var seatsJSON, seatsArJSON string
		if err := rows.Scan(&p.ID, &p.AccountID, &p.PaidOn, &p.Amount, &p.Currency,
			&p.Note, &p.AttachmentID, &p.Source, &p.CreatedAt, &p.AttachmentName,
			&p.Service, &p.CategoryID, &p.CategoryName, &p.CategoryIcon, &p.BillingPeriodMonths,
			&seatsJSON, &seatsArJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(seatsJSON), &p.Seats); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(seatsArJSON), &p.SeatsAr); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Create inserts a manual payment and returns its id.
func (s *PaymentStore) Create(ctx context.Context, p *Payment) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO payment (account_id, paid_on, amount, currency, note, attachment_id, source)
		 VALUES ($1, $2, $3, $4, $5, $6, 'manual')
		 RETURNING id`,
		p.AccountID, dateParam(&p.PaidOn), p.Amount, p.Currency, p.Note, p.AttachmentID,
	).Scan(&id)
	return id, err
}

// Update replaces the editable fields of a payment (source is never changed).
func (s *PaymentStore) Update(ctx context.Context, p *Payment) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE payment SET
		   paid_on = $2, amount = $3, currency = $4, note = $5, attachment_id = $6
		 WHERE id = $1`,
		p.ID, dateParam(&p.PaidOn), p.Amount, p.Currency, p.Note, p.AttachmentID)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a payment by id.
func (s *PaymentStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM payment WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// AccountOf returns the account a payment belongs to (for scoping edits/links).
func (s *PaymentStore) AccountOf(ctx context.Context, id string) (string, error) {
	var acc string
	err := s.db.QueryRowContext(ctx, `SELECT account_id FROM payment WHERE id = $1`, id).Scan(&acc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return acc, err
}
