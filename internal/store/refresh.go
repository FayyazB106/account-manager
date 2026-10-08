package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Refresh tokens back the long-lived, httpOnly-cookie session. The cookie holds
// an opaque "<id>.<secret>"; only sha256(secret) is stored. Tokens are rotated on
// every use — the spent row is kept (marked used) so replay of an old token is
// detected as theft and revokes the whole family.

var (
	// ErrRefreshInvalid: token missing, malformed, unknown, or expired.
	ErrRefreshInvalid = errors.New("invalid refresh token")
	// ErrRefreshReuse: a rotated (already-used) token was presented again —
	// treated as compromise; all of that admin's sessions are revoked.
	ErrRefreshReuse = errors.New("refresh token reuse detected")
)

type RefreshStore struct {
	db *sql.DB
}

func NewRefreshStore(db *sql.DB) *RefreshStore {
	return &RefreshStore{db: db}
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Issue creates a new refresh token for an admin and returns the cookie value
// ("<id>.<secret>"). The caller sets it as an httpOnly cookie.
func (s *RefreshStore) Issue(ctx context.Context, adminID string, ttl time.Duration) (string, error) {
	id, err := randHex(16)
	if err != nil {
		return "", err
	}
	secret, err := randHex(32)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO refresh_token (id, admin_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		id, adminID, hashSecret(secret), tsParam(time.Now().Add(ttl)))
	if err != nil {
		return "", err
	}
	return id + "." + secret, nil
}

// Rotate validates a presented cookie value, and on success spends it (marks it
// used) and issues a fresh token — returning the admin ID and the new cookie
// value. Reuse of an already-spent token revokes all of that admin's tokens.
func (s *RefreshStore) Rotate(ctx context.Context, cookie string, ttl time.Duration) (adminID, newCookie string, err error) {
	id, secret, ok := splitToken(cookie)
	if !ok {
		return "", "", ErrRefreshInvalid
	}

	var (
		storedHash []byte
		expiresAt  string
		usedAt     sql.NullString
	)
	row := s.db.QueryRowContext(ctx,
		`SELECT admin_id, token_hash, expires_at, used_at FROM refresh_token WHERE id = $1`, id)
	if err := row.Scan(&adminID, &storedHash, &expiresAt, &usedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrRefreshInvalid
		}
		return "", "", err
	}

	// Constant-time compare guards against timing oracles on the secret.
	if subtle.ConstantTimeCompare(storedHash, hashSecret(secret)) != 1 {
		return "", "", ErrRefreshInvalid
	}

	// Already spent → replay of a stolen/old token: revoke the whole family.
	if usedAt.Valid {
		_ = s.RevokeAllForAdmin(ctx, adminID)
		return "", "", ErrRefreshReuse
	}

	if parseSQLiteTS(expiresAt).Before(time.Now()) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM refresh_token WHERE id = $1`, id)
		return "", "", ErrRefreshInvalid
	}

	// Spend this token and mint a replacement in one transaction.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`UPDATE refresh_token SET used_at = $2 WHERE id = $1`, id, tsParam(time.Now())); err != nil {
		return "", "", err
	}
	newID, err := randHex(16)
	if err != nil {
		return "", "", err
	}
	newSecret, err := randHex(32)
	if err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO refresh_token (id, admin_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`,
		newID, adminID, hashSecret(newSecret), tsParam(time.Now().Add(ttl))); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return adminID, newID + "." + newSecret, nil
}

// Revoke deletes a single token (logout of this session). A missing/invalid
// cookie is a no-op.
func (s *RefreshStore) Revoke(ctx context.Context, cookie string) error {
	id, _, ok := splitToken(cookie)
	if !ok {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM refresh_token WHERE id = $1`, id)
	return err
}

// RevokeAllForAdmin deletes every token for an admin ("sign out everywhere",
// also used on password change and reuse detection).
func (s *RefreshStore) RevokeAllForAdmin(ctx context.Context, adminID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM refresh_token WHERE admin_id = $1`, adminID)
	return err
}

// Prune removes expired tokens and spent tokens older than a grace window (kept
// briefly so reuse can still be detected shortly after rotation).
func (s *RefreshStore) Prune(ctx context.Context) error {
	now := tsParam(time.Now())
	graceCutoff := tsParam(time.Now().Add(-24 * time.Hour))
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM refresh_token WHERE expires_at < $1 OR (used_at IS NOT NULL AND used_at < $2)`,
		now, graceCutoff)
	return err
}

func splitToken(cookie string) (id, secret string, ok bool) {
	i := strings.IndexByte(cookie, '.')
	if i <= 0 || i == len(cookie)-1 {
		return "", "", false
	}
	return cookie[:i], cookie[i+1:], true
}
