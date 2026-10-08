package crypto

import (
	"bytes"
	"testing"
)

// Envelope round-trip: encrypt then decrypt returns the original.
func TestEnvelopeRoundTrip(t *testing.T) {
	masterKey, _ := RandomBytes(KeySize)
	plaintext := []byte("adobe-account-password")

	env, err := EnvelopeEncrypt(masterKey, plaintext)
	if err != nil {
		t.Fatalf("EnvelopeEncrypt: %v", err)
	}

	// Neither stored field may contain the plaintext.
	if bytes.Contains(env.Ciphertext, plaintext) || bytes.Contains(env.WrappedDEK, plaintext) {
		t.Fatal("envelope leaks plaintext!")
	}

	got, err := EnvelopeDecrypt(masterKey, env)
	if err != nil {
		t.Fatalf("EnvelopeDecrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plaintext)
	}
}

// Without the master key, the stored envelope cannot be decrypted — this is the
// "DB leak alone is useless" property.
func TestEnvelopeWrongMasterKeyFails(t *testing.T) {
	masterKey, _ := RandomBytes(KeySize)
	wrongKey, _ := RandomBytes(KeySize)

	env, _ := EnvelopeEncrypt(masterKey, []byte("secret"))

	if _, err := EnvelopeDecrypt(wrongKey, env); err == nil {
		t.Fatal("expected decryption without the correct master key to fail")
	}
}

// Each secret gets its own DEK, so encrypting the same plaintext twice yields
// completely different envelopes.
func TestEnvelopeUniquePerSecret(t *testing.T) {
	masterKey, _ := RandomBytes(KeySize)

	a, _ := EnvelopeEncrypt(masterKey, []byte("same"))
	b, _ := EnvelopeEncrypt(masterKey, []byte("same"))

	if bytes.Equal(a.WrappedDEK, b.WrappedDEK) {
		t.Fatal("two secrets share a wrapped DEK (DEK reuse!)")
	}
	if bytes.Equal(a.Ciphertext, b.Ciphertext) {
		t.Fatal("identical plaintexts produced identical ciphertexts")
	}
}
