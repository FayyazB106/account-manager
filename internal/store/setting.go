package store

import (
	"context"
	"database/sql"
)

// SettingStore holds shared key/value app settings.
type SettingStore struct {
	db *sql.DB
}

func NewSettingStore(db *sql.DB) *SettingStore {
	return &SettingStore{db: db}
}

// All returns every setting as a key→value map.
func (s *SettingStore) All(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM app_setting`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Set upserts a setting.
func (s *SettingStore) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_setting (key, value) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		key, value)
	return err
}
