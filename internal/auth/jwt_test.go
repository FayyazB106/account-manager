package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestIssueAndParse(t *testing.T) {
	secret := []byte("test-secret")
	adminID := "abc-123"

	token, err := IssueToken(adminID, secret)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	got, err := ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if got != adminID {
		t.Fatalf("subject mismatch: got %q want %q", got, adminID)
	}
}

// A token signed with one secret must NOT verify under a different secret.
func TestWrongSecretRejected(t *testing.T) {
	token, _ := IssueToken("abc-123", []byte("real-secret"))

	if _, err := ParseToken(token, []byte("attacker-secret")); err == nil {
		t.Fatal("token verified under the wrong secret")
	}
}

// An expired token must be rejected.
func TestExpiredTokenRejected(t *testing.T) {
	secret := []byte("test-secret")

	// Build a token that already expired an hour ago.
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "abc-123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)

	if _, err := ParseToken(token, secret); err == nil {
		t.Fatal("expired token was accepted")
	}
}
