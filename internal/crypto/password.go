package crypto

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// This file handles password hashing for LOGIN (authentication) — a separate job
// from DeriveKey, which produces an encryption key. Login hashes are STORED and
// verified; encryption keys are never stored, only re-derived.
//
// Hashes are encoded in PHC string format, which embeds the algorithm,
// parameters, salt, and hash together:
//   $argon2id$v=19$m=65536,t=1,p=4$<base64 salt>$<base64 hash>
// This makes each hash self-describing, so verification still works even if we
// later change the cost parameters.

// HashPassword hashes a password with Argon2id and a fresh random salt, and
// returns the encoded PHC string to store in the database.
func HashPassword(password string) (string, error) {
	salt, err := RandomBytes(SaltSize)
	if err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, KeySize)

	b64 := base64.RawStdEncoding
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64.EncodeToString(salt), b64.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword reports whether password matches the stored PHC-encoded hash.
// It re-derives the hash using the SAME parameters and salt embedded in the
// encoded string, then compares in constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("invalid hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("parse version: %w", err)
	}

	var mem, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &time, &threads); err != nil {
		return false, fmt.Errorf("parse params: %w", err)
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}

	got := argon2.IDKey([]byte(password), salt, time, mem, threads, uint32(len(want)))

	// subtle.ConstantTimeCompare avoids leaking, via timing, how much of the
	// hash matched. Always compare secrets in constant time.
	if subtle.ConstantTimeCompare(got, want) == 1 {
		return true, nil
	}
	return false, nil
}
