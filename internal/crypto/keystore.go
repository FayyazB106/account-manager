package crypto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// This file adds "sealing": storing the server master key on disk encrypted with
// a passphrase, instead of in a plaintext environment variable. On startup the
// operator types the passphrase, which derives a Key-Encryption-Key (Argon2id)
// that unwraps the master key. The passphrase and the derived KEK are never
// stored — only the sealed blob is — so possession of the disk alone is useless.

const sealedKeyVersion = 1

// SealedKey is the on-disk, passphrase-protected form of the master key.
type SealedKey struct {
	Version int    `json:"version"`
	Salt    string `json:"salt"` // base64 Argon2id salt for the KEK
	Blob    string `json:"blob"` // base64 AES-256-GCM(master key) under the KEK
}

// SealKey encrypts the master key with a KEK derived from the passphrase.
func SealKey(master []byte, passphrase string) (*SealedKey, error) {
	if len(master) != KeySize {
		return nil, fmt.Errorf("master key must be %d bytes, got %d", KeySize, len(master))
	}
	salt, err := RandomBytes(SaltSize)
	if err != nil {
		return nil, err
	}
	kek := DeriveKey(passphrase, salt)
	blob, err := Encrypt(kek, master)
	if err != nil {
		return nil, err
	}
	return &SealedKey{
		Version: sealedKeyVersion,
		Salt:    base64.StdEncoding.EncodeToString(salt),
		Blob:    base64.StdEncoding.EncodeToString(blob),
	}, nil
}

// UnsealKey recovers the master key from a sealed key using the passphrase.
// A wrong passphrase fails authentication (GCM), returning a clear error.
func UnsealKey(sk *SealedKey, passphrase string) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(sk.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	blob, err := base64.StdEncoding.DecodeString(sk.Blob)
	if err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}
	kek := DeriveKey(passphrase, salt)
	master, err := Decrypt(kek, blob)
	if err != nil {
		return nil, errors.New("wrong passphrase")
	}
	if len(master) != KeySize {
		return nil, errors.New("unsealed key has wrong size")
	}
	return master, nil
}

// LoadSealedKeyFile reads a sealed key from disk. Callers can test the returned
// error with os.IsNotExist to detect a first run.
func LoadSealedKeyFile(path string) (*SealedKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sk SealedKey
	if err := json.Unmarshal(data, &sk); err != nil {
		return nil, fmt.Errorf("parse key file %s: %w", path, err)
	}
	return &sk, nil
}

// SaveSealedKeyFile writes a sealed key with owner-only permissions.
func SaveSealedKeyFile(path string, sk *SealedKey) error {
	data, err := json.MarshalIndent(sk, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
