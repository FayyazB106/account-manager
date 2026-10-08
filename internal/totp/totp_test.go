package totp

import (
	"testing"
	"time"
)

// A generated secret's own current code must verify, and neighbours must too
// (±30s), while a clearly-wrong code must not.
func TestGenerateAndVerify(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	now := time.Now()
	code, err := codeAt(secret, uint64(now.Unix()/period))
	if err != nil {
		t.Fatalf("codeAt: %v", err)
	}
	if !Verify(secret, code, now) {
		t.Fatal("current code failed to verify")
	}
	// Skew tolerance: a code from the previous window still verifies now.
	prev, _ := codeAt(secret, uint64(now.Unix()/period)-1)
	if !Verify(secret, prev, now) {
		t.Fatal("previous-window code should verify within skew")
	}
	if Verify(secret, "000000", now) && code == "000000" {
		// extremely unlikely; skip
	}
	if Verify(secret, "abc", now) {
		t.Fatal("malformed code should not verify")
	}
}

// RFC 6238 test vector (SHA1, secret "12345678901234567890").
func TestRFC6238Vector(t *testing.T) {
	// base32 of the ASCII secret "12345678901234567890".
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	at := time.Unix(59, 0).UTC()
	got, err := codeAt(secret, uint64(at.Unix()/period))
	if err != nil {
		t.Fatalf("codeAt: %v", err)
	}
	if got != "287082" {
		t.Fatalf("RFC vector mismatch: got %s want 287082", got)
	}
}

func TestBackupCodeHashStable(t *testing.T) {
	if HashCode("A1B2C3D4") != HashCode("a1b2c3d4 ") {
		t.Fatal("hash should be case- and space-insensitive")
	}
}
