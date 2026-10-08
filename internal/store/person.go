package store

import (
	"context"
	"database/sql"
	"errors"
)

// Person is someone who occupies account seats (not an admin/login).
type Person struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	NameAr *string `json:"name_ar,omitempty"`
}

type PersonStore struct {
	db *sql.DB
}

func NewPersonStore(db *sql.DB) *PersonStore {
	return &PersonStore{db: db}
}

func (s *PersonStore) List(ctx context.Context) ([]Person, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, name_ar FROM person ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.Name, &p.NameAr); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PersonStore) Create(ctx context.Context, name string, nameAr *string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `INSERT INTO person (name, name_ar) VALUES ($1, $2) RETURNING id`, name, nameAr).Scan(&id)
	return id, err
}

// PersonInput is one row for a bulk import (English name + optional Arabic).
type PersonInput struct {
	Name   string
	NameAr *string
}

// CreateMany inserts several people in one transaction (used by Excel import).
func (s *PersonStore) CreateMany(ctx context.Context, people []PersonInput) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, p := range people {
		if _, err := tx.ExecContext(ctx, `INSERT INTO person (name, name_ar) VALUES ($1, $2)`, p.Name, p.NameAr); err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}

// Update renames a person and propagates the new name to any account seats that
// referenced them (seats store the name as a snapshot), so a rename is reflected
// everywhere the person is assigned.
func (s *PersonStore) Update(ctx context.Context, id, name string, nameAr *string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldName string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM person WHERE id = $1`, id).Scan(&oldName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE person SET name = $2, name_ar = $3 WHERE id = $1`, id, name, nameAr); err != nil {
		return err
	}
	// Keep seats in sync: rows that held the old name get the new name (and the
	// new Arabic name). Blank seats (from a deleted person) have user_name = ''
	// and are left untouched.
	if _, err := tx.ExecContext(ctx,
		`UPDATE account_user SET user_name = $2, user_name_ar = $3 WHERE user_name = $1 AND user_name <> ''`,
		oldName, name, nameAr); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a person and blanks any account seats that hold their name, so
// the seat slots remain but no longer reference the deleted person.
func (s *PersonStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := deletePersonTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteMany removes several people (and blanks their seats) in one transaction.
// Returns the number deleted. Missing ids are skipped.
func (s *PersonStore) DeleteMany(ctx context.Context, ids []string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, id := range ids {
		err := deletePersonTx(ctx, tx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}

// deletePersonTx deletes one person within a transaction, blanking any seats
// that reference them by name.
func deletePersonTx(ctx context.Context, tx *sql.Tx, id string) error {
	var name string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM person WHERE id = $1`, id).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE account_user SET user_name = '', user_name_ar = NULL WHERE user_name = $1`, name); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM person WHERE id = $1`, id)
	return err
}
