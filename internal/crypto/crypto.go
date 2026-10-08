// Package crypto holds the low-level cryptographic primitives used across the
// application: secure random bytes, key derivation (Argon2id), and authenticated
// encryption (AES-256-GCM).
//
// These are deliberately small, well-documented building blocks. Higher-level
// logic (envelope encryption, the login flow) is built ON TOP of these in other
// files, so the dangerous parts live in exactly one, well-tested place.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// KeySize is the AES-256 key length in bytes (256 bits / 8).
const KeySize = 32

// Argon2id parameters. These control how slow/expensive key derivation is.
// Higher = more resistant to brute force, but slower for legitimate users.
// These are sensible modern defaults; tune per deployment.
const (
	argonTime    = 1         // number of passes
	argonMemory  = 64 * 1024 // memory in KiB (64 MiB)
	argonThreads = 4         // parallelism
	SaltSize     = 16        // salt length in bytes
)

// RandomBytes returns n cryptographically secure random bytes, using the OS
// CSPRNG (crypto/rand). NEVER use math/rand for keys, salts, nonces, or
// generated passwords — it is predictable.
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("random bytes: %w", err)
	}
	return b, nil
}

// DeriveKey turns a password + salt into a fixed-size 32-byte key using
// Argon2id. The same password+salt always yields the same key (so we can
// re-derive it at login), but it is deliberately expensive to compute, which is
// what defeats brute-force attacks on a leaked database.
func DeriveKey(password string, salt []byte) []byte {
	return argon2.IDKey(
		[]byte(password),
		salt,
		argonTime,
		argonMemory,
		argonThreads,
		KeySize,
	)
}

// Encrypt performs AES-256-GCM authenticated encryption. It returns the
// nonce prepended to the ciphertext: nonce || ciphertext.
//
// "Authenticated" (the GCM part) means decryption will FAIL if the data was
// tampered with — you get confidentiality AND integrity together. A fresh random
// nonce is generated per call; reusing a nonce with the same key is catastrophic,
// so we never do.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", KeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	// Seal appends the ciphertext to its first arg (the nonce), so the result is
	// nonce || ciphertext || auth-tag.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt. It expects nonce || ciphertext and returns the
// original plaintext, or an error if the key is wrong or the data was tampered
// with (GCM authentication failure).
func Decrypt(key, blob []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", KeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(blob) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := blob[:nonceSize], blob[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Wrong key OR tampered data both land here — we don't leak which.
		return nil, errors.New("decryption failed")
	}
	return plaintext, nil
}
