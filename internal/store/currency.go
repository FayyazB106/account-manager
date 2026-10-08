package store

import (
	"context"
	"database/sql"
	"time"
)

type Currency struct {
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Active    bool       `json:"active"`
	Rate      float64    `json:"rate"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type CurrencyStore struct {
	db *sql.DB
}

func NewCurrencyStore(db *sql.DB) *CurrencyStore {
	return &CurrencyStore{db: db}
}

func (s *CurrencyStore) List(ctx context.Context) ([]Currency, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code, name, active, rate, updated_at FROM currency ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Currency
	for rows.Next() {
		var c Currency
		if err := rows.Scan(&c.Code, &c.Name, &c.Active, &c.Rate, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Codes returns all currency codes (used by the rate updater).
func (s *CurrencyStore) Codes(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code FROM currency`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *CurrencyStore) SetActive(ctx context.Context, code string, active bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE currency SET active = $2 WHERE code = $1`, code, active)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertRates inserts any new currency (inactive) and refreshes rate/updated_at
// for all. Existing rows keep their name and active flag.
func (s *CurrencyStore) UpsertRates(ctx context.Context, rates map[string]float64, name func(code string) string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := tsParam(time.Now())
	for code, rate := range rates {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO currency (code, name, active, rate, updated_at)
			 VALUES ($1, $2, 0, $3, $4)
			 ON CONFLICT (code) DO UPDATE SET rate = EXCLUDED.rate, updated_at = EXCLUDED.updated_at`,
			code, name(code), rate, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
