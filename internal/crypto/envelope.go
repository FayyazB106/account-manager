package crypto

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// Envelope is the stored result of encrypting one secret with envelope
// encryption. Both fields are safe to store in the database; neither is useful
// without the Master Key (which lives outside the DB).
type Envelope struct {
	WrappedDEK []byte // the per-secret Data Encryption Key, encrypted with the Master Key
	Ciphertext []byte // the secret, encrypted with the DEK
}

// EnvelopeEncrypt encrypts plaintext using a fresh per-secret DEK, then wraps
// that DEK with the master key. This is the server-side "outer lock".
//
//  1. generate a random DEK
//  2. encrypt the plaintext with the DEK          -> Ciphertext
//  3. encrypt (wrap) the DEK with the master key  -> WrappedDEK
func EnvelopeEncrypt(masterKey, plaintext []byte) (*Envelope, error) {
	dek, err := RandomBytes(KeySize)
	if err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}

	ciphertext, err := Encrypt(dek, plaintext)
	if err != nil {
		return nil, fmt.Errorf("encrypt plaintext: %w", err)
	}

	wrappedDEK, err := Encrypt(masterKey, dek)
	if err != nil {
		return nil, fmt.Errorf("wrap dek: %w", err)
	}

	return &Envelope{WrappedDEK: wrappedDEK, Ciphertext: ciphertext}, nil
}

// EnvelopeDecrypt reverses EnvelopeEncrypt: unwrap the DEK with the master key,
// then use the DEK to decrypt the ciphertext.
func EnvelopeDecrypt(masterKey []byte, e *Envelope) ([]byte, error) {
	dek, err := Decrypt(masterKey, e.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("unwrap dek: %w", err)
	}

	plaintext, err := Decrypt(dek, e.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return plaintext, nil
}

// SealString envelope-encrypts a short secret (e.g. an SMTP password) and returns
// it as one printable string safe to keep in a key/value setting:
// "v1:<base64 wrapped DEK>.<base64 ciphertext>".
func SealString(masterKey []byte, plaintext string) (string, error) {
	env, err := EnvelopeEncrypt(masterKey, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return "v1:" + base64.StdEncoding.EncodeToString(env.WrappedDEK) + "." +
		base64.StdEncoding.EncodeToString(env.Ciphertext), nil
}

// OpenString reverses SealString.
func OpenString(masterKey []byte, sealed string) (string, error) {
	body, ok := strings.CutPrefix(sealed, "v1:")
	if !ok {
		return "", fmt.Errorf("unrecognised sealed value")
	}
	dekPart, ctPart, ok := strings.Cut(body, ".")
	if !ok {
		return "", fmt.Errorf("malformed sealed value")
	}
	dek, err := base64.StdEncoding.DecodeString(dekPart)
	if err != nil {
		return "", fmt.Errorf("malformed sealed value: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(ctPart)
	if err != nil {
		return "", fmt.Errorf("malformed sealed value: %w", err)
	}
	pt, err := EnvelopeDecrypt(masterKey, &Envelope{WrappedDEK: dek, Ciphertext: ct})
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// LoadMasterKey reads the server master key from the MASTER_KEY environment
// variable, which holds a base64-encoded 32-byte key. Keeping the key in the
// environment (not the database, not the source code) is what makes a DB-only
// leak useless.
func LoadMasterKey() ([]byte, error) {
	raw := os.Getenv("MASTER_KEY")
	if raw == "" {
		return nil, fmt.Errorf("MASTER_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("MASTER_KEY is not valid base64: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("MASTER_KEY must decode to %d bytes, got %d", KeySize, len(key))
	}
	return key, nil
}
