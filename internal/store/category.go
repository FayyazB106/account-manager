package store

import (
	"context"
	"database/sql"
	"errors"
)

type Category struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	NameAr *string `json:"name_ar,omitempty"` // optional Arabic name
	Icon   string  `json:"icon"`
}

type CategoryStore struct {
	db *sql.DB
}

func NewCategoryStore(db *sql.DB) *CategoryStore {
	return &CategoryStore{db: db}
}

func (s *CategoryStore) List(ctx context.Context) ([]Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, name_ar, icon FROM category ORDER BY position, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.NameAr, &c.Icon); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *CategoryStore) Create(ctx context.Context, name string, nameAr *string, icon string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO category (name, name_ar, icon) VALUES ($1, $2, $3) RETURNING id`,
		name, nameAr, icon,
	).Scan(&id)
	return id, err
}

func (s *CategoryStore) Update(ctx context.Context, id, name string, nameAr *string, icon string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE category SET name = $2, name_ar = $3, icon = $4 WHERE id = $1`, id, name, nameAr, icon)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeArabicNames clears every Arabic name across the tables that carry one.
// Used when an admin turns off Arabic support and opts to delete Arabic data.
func (s *CategoryStore) PurgeArabicNames(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{
		`UPDATE category SET name_ar = NULL`,
		`UPDATE account_user SET user_name_ar = NULL`,
		`UPDATE admin SET name_ar = NULL`,
		`UPDATE person SET name_ar = NULL`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ErrCategoryInUse means a delete was attempted on a category that still has
// accounts assigned to it.
var ErrCategoryInUse = errors.New("category still has accounts")

func (s *CategoryStore) Delete(ctx context.Context, id string) error {
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM account WHERE category_id = $1`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrCategoryInUse
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM category WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// Reorder sets each category's position to its index in the given id list,
// atomically.
func (s *CategoryStore) Reorder(ctx context.Context, ids []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE category SET position = $2 WHERE id = $1`, id, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}
