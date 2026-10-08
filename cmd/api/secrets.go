package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/FayyazB106/account-manager/internal/config"
	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

const minAdminPasswordLen = 12

const minPassphraseLen = 8

// readSecret reads a passphrase without echoing it. For non-interactive use
// (scripts, unattended servers, tests) it falls back to the MASTER_PASSPHRASE
// environment variable.
func readSecret(prompt string) (string, error) {
	if v := os.Getenv("MASTER_PASSPHRASE"); v != "" {
		return v, nil
	}
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", errors.New("no terminal available for the passphrase prompt; set MASTER_PASSPHRASE for non-interactive use")
	}
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// resolveStartupKey returns the server master key at startup WITHOUT any
// interactive console prompt. Priority:
//  1. MASTER_KEY env var (unattended server) — used as-is.
//  2. MASTER_PASSPHRASE env var — unseal an existing key file, or bootstrap a new
//     sealed key non-interactively (for CI/scripts).
//  3. Otherwise return nil — the app starts un-initialized/locked and the browser
//     handles first-run Setup (set passphrase + first admin) or Unlock.
func resolveStartupKey(cfg *config.Config) ([]byte, error) {
	if os.Getenv("MASTER_KEY") != "" {
		return crypto.LoadMasterKey()
	}

	sk, err := crypto.LoadSealedKeyFile(cfg.KeyPath)
	if err == nil {
		// Sealed key exists. Auto-unseal only with an env passphrase; otherwise
		// leave it locked for the browser Unlock page.
		if pass := os.Getenv("MASTER_PASSPHRASE"); pass != "" {
			return crypto.UnsealKey(sk, pass)
		}
		return nil, nil // locked
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	// No key yet. Bootstrap non-interactively if a passphrase is supplied via env;
	// otherwise leave it for the browser Setup page.
	if pass := os.Getenv("MASTER_PASSPHRASE"); pass != "" {
		key, err := crypto.RandomBytes(crypto.KeySize)
		if err != nil {
			return nil, err
		}
		if err := seal(cfg.KeyPath, key, pass); err != nil {
			return nil, err
		}
		return key, nil
	}
	return nil, nil // first-run setup
}

// sealMasterKeyCmd (invoked by -seal-master-key) takes the current MASTER_KEY
// from the environment and writes it out sealed under a new passphrase, so an
// existing deployment can move off the plaintext env var without re-encrypting
// any data. It refuses to overwrite an existing key file.
func sealMasterKeyCmd(cfg *config.Config) error {
	if _, err := os.Stat(cfg.KeyPath); err == nil {
		return fmt.Errorf("a key file already exists at %s; remove it first to re-seal", cfg.KeyPath)
	}
	key, err := crypto.LoadMasterKey()
	if err != nil {
		return fmt.Errorf("MASTER_KEY must be set to seal it: %w", err)
	}
	pass, err := newPassphrase()
	if err != nil {
		return err
	}
	if err := seal(cfg.KeyPath, key, pass); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Master key sealed at %s.\nYou can now remove MASTER_KEY from the environment; you'll be prompted for the passphrase on startup.\n", cfg.KeyPath)
	return nil
}

// resetAdminCmd (invoked by -reset-admin) recovers a locked-out admin: it can
// reset their password and/or clear their two-factor, without hand-editing SQL.
// Listing admins and clearing 2FA need only DB access; resetting a password also
// requires the vault passphrase (personal mode) or MASTER_KEY (server mode) as
// proof of ownership, so a stolen DB file alone can't yield a working login.
// With no -email it lists the admins so you can pick one.
func resetAdminCmd(cfg *config.Config, email string, resetPw, clearMFA bool) error {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()
	admins := store.NewAdminStore(database)
	ctx := context.Background()

	if strings.TrimSpace(email) == "" {
		list, err := admins.List(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Admins:")
		for _, a := range list {
			fmt.Fprintf(os.Stderr, "  %s\n", a.Email)
		}
		fmt.Fprintln(os.Stderr, "\nRe-run with -email <email> and -reset-password and/or -clear-mfa.")
		return nil
	}

	a, err := admins.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no admin with email %q", email)
	}
	if err != nil {
		return err
	}
	if !resetPw && !clearMFA {
		return errors.New("nothing to do — pass -reset-password and/or -clear-mfa")
	}

	if clearMFA {
		if err := admins.DisableTOTP(ctx, a.ID); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Cleared two-factor for %s.\n", a.Email)
	}
	if resetPw {
		// Require the vault passphrase as proof of ownership before resetting a
		// password, so someone with only the DB file (or a backup) can't reset an
		// admin and gain a working login. When a sealed key file exists (personal
		// mode) we verify by unsealing it. In MASTER_KEY server mode the env key is
		// already the proof, so we skip. (MASTER_PASSPHRASE, if set, satisfies the
		// prompt non-interactively via readSecret.)
		if os.Getenv("MASTER_KEY") == "" {
			sk, err := crypto.LoadSealedKeyFile(cfg.KeyPath)
			if err == nil {
				pass, perr := readSecret("Vault passphrase: ")
				if perr != nil {
					return perr
				}
				if _, uerr := crypto.UnsealKey(sk, pass); uerr != nil {
					return errors.New("incorrect passphrase")
				}
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("read key file: %w", err)
			}
			// No sealed key file and no MASTER_KEY: vault isn't initialized yet, so
			// there's no passphrase to verify against — allow the reset.
		}

		pw, err := newAdminPassword()
		if err != nil {
			return err
		}
		authHash, err := crypto.HashPassword(pw)
		if err != nil {
			return err
		}
		kekSalt, err := crypto.RandomBytes(crypto.SaltSize)
		if err != nil {
			return err
		}
		// The per-admin wrapped Vault Key is scaffolding (not used for decryption
		// in the current server-envelope model), but we regenerate it to keep the
		// row well-formed.
		kek := crypto.DeriveKey(pw, kekSalt)
		vaultKey, err := crypto.RandomBytes(crypto.KeySize)
		if err != nil {
			return err
		}
		wrapped, err := crypto.Encrypt(kek, vaultKey)
		if err != nil {
			return err
		}
		if err := admins.UpdateCredentials(ctx, a.ID, authHash, kekSalt, wrapped); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Password reset for %s.\n", a.Email)
	}
	return nil
}

// newAdminPassword prompts for a new admin password twice.
func newAdminPassword() (string, error) {
	pw, err := readSecret("New password: ")
	if err != nil {
		return "", err
	}
	if len(pw) < minAdminPasswordLen {
		return "", fmt.Errorf("password must be at least %d characters", minAdminPasswordLen)
	}
	confirm, err := readSecret("Confirm password: ")
	if err != nil {
		return "", err
	}
	if pw != confirm {
		return "", errors.New("passwords do not match")
	}
	return pw, nil
}

// genKeyCmd (invoked by -gen-key) prints a fresh random base64 MASTER_KEY, for
// env-based unattended deployments (set it as MASTER_KEY in the environment or a
// .env file). It generates and prints only — it stores nothing.
func genKeyCmd() error {
	k, err := crypto.RandomBytes(crypto.KeySize)
	if err != nil {
		return err
	}
	fmt.Println(base64.StdEncoding.EncodeToString(k))
	return nil
}

// changePassphraseCmd (invoked by -change-passphrase) re-seals the existing
// master key under a new passphrase. It never touches the key value itself, so
// no data is re-encrypted; it only requires the current passphrase.
func changePassphraseCmd(cfg *config.Config) error {
	sk, err := crypto.LoadSealedKeyFile(cfg.KeyPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("no sealed key file at %s — nothing to change (seal one first with -seal-master-key)", cfg.KeyPath)
	}
	if err != nil {
		return err
	}
	current, err := readSecret("Current passphrase: ")
	if err != nil {
		return err
	}
	key, err := crypto.UnsealKey(sk, current)
	if err != nil {
		return fmt.Errorf("current passphrase is incorrect")
	}
	newPass, err := newPassphrase()
	if err != nil {
		return err
	}
	if err := seal(cfg.KeyPath, key, newPass); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Passphrase changed. Use the new passphrase on the next startup.")
	return nil
}

// newPassphrase prompts for a passphrase twice and checks they match. It re-prompts
// on a too-short or mismatched entry (rather than exiting) so a typo in the hidden
// prompt doesn't drop the operator back to a relaunch loop. A genuine read error
// (no interactive terminal) still returns, so unattended use fails fast.
func newPassphrase() (string, error) {
	for {
		pass, err := readSecret(fmt.Sprintf("Create a vault passphrase (min %d characters): ", minPassphraseLen))
		if err != nil {
			return "", err
		}
		if len(pass) < minPassphraseLen {
			fmt.Fprintln(os.Stderr, "  too short — try again")
			continue
		}
		confirm, err := readSecret("Confirm passphrase: ")
		if err != nil {
			return "", err
		}
		if pass != confirm {
			fmt.Fprintln(os.Stderr, "  passphrases did not match — try again")
			continue
		}
		return pass, nil
	}
}

func seal(path string, key []byte, passphrase string) error {
	sk, err := crypto.SealKey(key, passphrase)
	if err != nil {
		return err
	}
	return crypto.SaveSealedKeyFile(path, sk)
}

// resolveJWTSecret returns the JWT signing secret: JWT_SECRET if set, otherwise a
// persistent random secret stored under the data dir (so sessions survive
// restarts without any env configuration).
func resolveJWTSecret(cfg *config.Config) ([]byte, error) {
	if cfg.JWTSecret != "" {
		return []byte(cfg.JWTSecret), nil
	}
	path := cfg.DataDir + string(os.PathSeparator) + "jwt.secret"
	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read jwt secret: %w", err)
	}
	raw, err := crypto.RandomBytes(crypto.KeySize)
	if err != nil {
		return nil, err
	}
	secret := []byte(base64.StdEncoding.EncodeToString(raw))
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, fmt.Errorf("write jwt secret: %w", err)
	}
	return secret, nil
}
