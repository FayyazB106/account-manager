// Package totp implements time-based one-time passwords (RFC 6238) for admin
// login MFA, plus single-use backup codes. It uses only the standard library —
// the same 6-digit/30-second SHA1 scheme every authenticator app (Google
// Authenticator, Authy, ...) speaks, so no external service or SDK is involved.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	period      = 30 // seconds per code
	digits      = 6
	secretBytes = 20 // 160-bit secret, the RFC-recommended size
)

// base32 without padding, the encoding authenticator apps expect for the secret.
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new random base32 TOTP secret.
func GenerateSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// codeAt computes the 6-digit code for a specific 30-second counter value.
func codeAt(secret string, counter uint64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 §5.3).
	offset := sum[len(sum)-1] & 0x0f
	val := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)
	return fmt.Sprintf("%0*d", digits, val%1_000_000), nil
}

// Code returns the current 6-digit code for a secret plus the seconds left in
// its 30-second window (for a countdown). Used to display 2FA codes for saved
// accounts.
func Code(secret string, t time.Time) (code string, secondsRemaining int, err error) {
	unix := t.Unix()
	code, err = codeAt(secret, uint64(unix)/period)
	if err != nil {
		return "", 0, err
	}
	return code, period - int(unix%period), nil
}

// ParseSecret accepts either a base32 secret or a full otpauth:// URI and returns
// the normalized base32 secret, validating that it decodes.
func ParseSecret(input string) (string, error) {
	s := strings.TrimSpace(input)
	if strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", errors.New("invalid otpauth URI")
		}
		s = u.Query().Get("secret")
		if s == "" {
			return "", errors.New("otpauth URI has no secret")
		}
	}
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	if s == "" {
		return "", errors.New("empty secret")
	}
	if _, err := b32.DecodeString(s); err != nil {
		return "", errors.New("invalid TOTP secret")
	}
	return s, nil
}

// Verify reports whether input matches the secret's code, allowing a ±1 window
// (±30s) for clock skew between the server and the authenticator app.
func Verify(secret, input string, t time.Time) bool {
	input = strings.TrimSpace(input)
	if len(input) != digits {
		return false
	}
	counter := int64(t.Unix()) / period
	for d := int64(-1); d <= 1; d++ {
		c, err := codeAt(secret, uint64(counter+d))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(c), []byte(input)) == 1 {
			return true
		}
	}
	return false
}

// OtpauthURL builds the otpauth:// provisioning URI that a QR code encodes.
func OtpauthURL(secret, account, issuer string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", digits))
	q.Set("period", fmt.Sprintf("%d", period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// --- backup codes -----------------------------------------------------------

// GenerateBackupCode returns a random single-use recovery code (8 hex chars).
func GenerateBackupCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashCode hashes a backup code for storage. Backup codes are high-entropy
// random values, so a fast SHA-256 is appropriate (unlike user passwords).
func HashCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(code))))
	return hex.EncodeToString(sum[:])
}
