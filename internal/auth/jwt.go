// Package auth issues and validates session tokens (JWTs) used to prove an admin
// has already logged in, so they don't resend their password on every request.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenTTL is how long an access token stays valid. It's deliberately short: the
// client silently renews it from the httpOnly refresh-token cookie (see the
// session/refresh flow), so a stolen access token is only briefly useful.
const TokenTTL = 15 * time.Minute

// MFATokenTTL is how long the intermediate "password accepted, awaiting code"
// token is valid — just long enough to type an authenticator code.
const MFATokenTTL = 5 * time.Minute

// Token scopes distinguish a full session from the short-lived MFA-pending
// token, so the latter can never be used to call the API.
const (
	scopeSession = "session"
	scopeMFA     = "mfa"
)

// Claims is the JWT payload. RegisteredClaims gives us standard fields like
// Subject (who), ExpiresAt, and IssuedAt. Scope marks a full session vs an
// MFA-pending token. Nothing secret goes here — a JWT payload is readable.
type Claims struct {
	Scope string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

// IssueToken creates a full session token for the given admin ID, valid for
// TokenTTL. Without the server secret the token cannot be forged.
func IssueToken(adminID string, secret []byte) (string, error) {
	return issue(adminID, scopeSession, TokenTTL, secret)
}

// IssueMFAToken creates the short-lived token returned after the password step
// when MFA is enabled; it is exchanged for a session token via the code step.
func IssueMFAToken(adminID string, secret []byte) (string, error) {
	return issue(adminID, scopeMFA, MFATokenTTL, secret)
}

func issue(adminID, scope string, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	claims := Claims{
		Scope: scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   adminID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	// HS256 = HMAC-SHA256: a symmetric signature using our shared secret.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// ParseToken verifies a full SESSION token and returns the admin ID. An
// MFA-pending token is rejected here.
func ParseToken(tokenString string, secret []byte) (string, error) {
	return parseScoped(tokenString, scopeSession, secret)
}

// ParseMFAToken verifies an MFA-pending token and returns the admin ID.
func ParseMFAToken(tokenString string, secret []byte) (string, error) {
	return parseScoped(tokenString, scopeMFA, secret)
}

func parseScoped(tokenString, wantScope string, secret []byte) (string, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		// Reject any token whose algorithm isn't the HMAC we expect. This blocks
		// the classic "alg=none" and algorithm-confusion forgery attacks.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("invalid token")
	}
	// A missing scope is treated as a session token (back-compat with tokens
	// issued before scopes existed).
	got := claims.Scope
	if got == "" {
		got = scopeSession
	}
	if got != wantScope {
		return "", errors.New("wrong token scope")
	}
	return claims.Subject, nil
}
