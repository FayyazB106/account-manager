package crypto

import (
	"strings"
	"testing"
)

func TestHashPasswordFormat(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	// The plaintext password must never appear in the stored hash.
	if strings.Contains(hash, "staple") {
		t.Fatal("hash leaks the password")
	}
}

func TestVerifyCorrectPassword(t *testing.T) {
	hash, _ := HashPassword("s3cret-master-pw")

	ok, err := VerifyPassword("s3cret-master-pw", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("correct password was rejected")
	}
}

func TestVerifyWrongPassword(t *testing.T) {
	hash, _ := HashPassword("s3cret-master-pw")

	ok, _ := VerifyPassword("wrong-password", hash)
	if ok {
		t.Fatal("wrong password was accepted")
	}
}

// The same password hashed twice must produce DIFFERENT stored hashes, because
// each uses a fresh random salt. Both must still verify.
func TestHashesAreSalted(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")

	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical (salt not random!)")
	}

	if ok, _ := VerifyPassword("same", h1); !ok {
		t.Fatal("h1 failed to verify")
	}
	if ok, _ := VerifyPassword("same", h2); !ok {
		t.Fatal("h2 failed to verify")
	}
}
