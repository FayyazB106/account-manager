package crypto

import (
	"bytes"
	"testing"
)

// A round-trip: encrypting then decrypting must return the original plaintext.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := RandomBytes(KeySize)
	if err != nil {
		t.Fatalf("RandomBytes: %v", err)
	}

	plaintext := []byte("my-super-secret-password")

	blob, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// The stored blob must NOT contain the plaintext anywhere.
	if bytes.Contains(blob, plaintext) {
		t.Fatal("ciphertext leaks the plaintext!")
	}

	got, err := Decrypt(key, blob)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plaintext)
	}
}

// Decrypting with the wrong key must fail, not return garbage.
func TestDecryptWrongKeyFails(t *testing.T) {
	key, _ := RandomBytes(KeySize)
	wrongKey, _ := RandomBytes(KeySize)

	blob, _ := Encrypt(key, []byte("secret"))

	if _, err := Decrypt(wrongKey, blob); err == nil {
		t.Fatal("expected decryption with wrong key to fail, but it succeeded")
	}
}

// Tampering with the ciphertext must be detected (GCM authentication).
func TestTamperingDetected(t *testing.T) {
	key, _ := RandomBytes(KeySize)
	blob, _ := Encrypt(key, []byte("secret"))

	blob[len(blob)-1] ^= 0xFF // flip the last byte

	if _, err := Decrypt(key, blob); err == nil {
		t.Fatal("expected tampered ciphertext to be rejected, but it was accepted")
	}
}

// Same password + same salt => same key (needed to re-derive at login).
// Same password + DIFFERENT salt => different key (defeats rainbow tables).
func TestDeriveKeyDeterministicAndSalted(t *testing.T) {
	saltA, _ := RandomBytes(SaltSize)
	saltB, _ := RandomBytes(SaltSize)

	k1 := DeriveKey("password", saltA)
	k2 := DeriveKey("password", saltA)
	k3 := DeriveKey("password", saltB)

	if !bytes.Equal(k1, k2) {
		t.Fatal("same password+salt should derive the same key")
	}
	if bytes.Equal(k1, k3) {
		t.Fatal("different salts should derive different keys")
	}
	if len(k1) != KeySize {
		t.Fatalf("derived key wrong size: got %d want %d", len(k1), KeySize)
	}
}

// Two encryptions of the same plaintext must differ (random nonce per call).
func TestNonceMakesCiphertextsUnique(t *testing.T) {
	key, _ := RandomBytes(KeySize)
	a, _ := Encrypt(key, []byte("same"))
	b, _ := Encrypt(key, []byte("same"))

	if bytes.Equal(a, b) {
		t.Fatal("identical plaintexts produced identical ciphertexts (nonce reuse!)")
	}
}
