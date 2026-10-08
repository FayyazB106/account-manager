package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AttachmentMeta is the secret-free view of an attachment (no file bytes).
type AttachmentMeta struct {
	ID        string    `json:"id"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
}

type AttachmentStore struct {
	db *sql.DB
}

func NewAttachmentStore(db *sql.DB) *AttachmentStore {
	return &AttachmentStore{db: db}
}

// Create stores an already-encrypted attachment for an account.
func (s *AttachmentStore) Create(ctx context.Context, accountID, filename, mime string, wrappedDEK, ciphertext []byte) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO attachment (account_id, filename, mime_type, wrapped_dek, ciphertext)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		accountID, filename, mime, wrappedDEK, ciphertext,
	).Scan(&id)
	return id, err
}

// ListByAccount returns attachment metadata (no bytes) for an account.
func (s *AttachmentStore) ListByAccount(ctx context.Context, accountID string) ([]AttachmentMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, filename, mime_type, created_at
		 FROM attachment WHERE account_id = $1 ORDER BY created_at DESC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AttachmentMeta
	for rows.Next() {
		var m AttachmentMeta
		var mime sql.NullString
		if err := rows.Scan(&m.ID, &m.Filename, &mime, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.MimeType = mime.String
		out = append(out, m)
	}
	return out, rows.Err()
}

// AccountOf returns the account an attachment belongs to, so a handler can verify
// an attachment being linked to a payment lives on the same account.
func (s *AttachmentStore) AccountOf(ctx context.Context, id string) (string, error) {
	var acc string
	err := s.db.QueryRowContext(ctx, `SELECT account_id FROM attachment WHERE id = $1`, id).Scan(&acc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return acc, err
}

// Delete removes an attachment by id.
func (s *AttachmentStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM attachment WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if affected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// AttachmentBlob is one attachment's filename + encrypted contents (for export).
type AttachmentBlob struct {
	Filename   string
	WrappedDEK []byte
	Ciphertext []byte
}

// ListBlobsByAccount returns every attachment's encrypted contents for an account.
func (s *AttachmentStore) ListBlobsByAccount(ctx context.Context, accountID string) ([]AttachmentBlob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT filename, wrapped_dek, ciphertext FROM attachment
		 WHERE account_id = $1 ORDER BY created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AttachmentBlob
	for rows.Next() {
		var b AttachmentBlob
		if err := rows.Scan(&b.Filename, &b.WrappedDEK, &b.Ciphertext); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetForDownload returns the encrypted blobs plus filename/mime for one attachment.
func (s *AttachmentStore) GetForDownload(ctx context.Context, id string) (filename, mime string, wrappedDEK, ciphertext []byte, err error) {
	var mimeNull sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT filename, mime_type, wrapped_dek, ciphertext FROM attachment WHERE id = $1`,
		id,
	).Scan(&filename, &mimeNull, &wrappedDEK, &ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil, nil, ErrNotFound
	}
	return filename, mimeNull.String, wrappedDEK, ciphertext, err
}
