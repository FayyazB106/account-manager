// Package passwords generates strong random passwords and estimates password
// strength (entropy). Generation uses a cryptographically secure RNG.
package passwords

import (
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"strings"
)

// Character classes a password can draw from.
const (
	lowercase = "abcdefghijklmnopqrstuvwxyz"
	uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits    = "0123456789"
	symbols   = "!@#$%^&*()-_=+[]{};:,.<>?"
)

// Options controls generation. Length is the number of characters; the booleans
// choose which classes to include.
type Options struct {
	Length     int
	UseLower   bool
	UseUpper   bool
	UseDigits  bool
	UseSymbols bool
}

// DefaultOptions is a strong, sensible default: 20 chars, all classes.
func DefaultOptions() Options {
	return Options{Length: 20, UseLower: true, UseUpper: true, UseDigits: true, UseSymbols: true}
}

// pool builds the character set from the selected classes.
func (o Options) pool() string {
	var b strings.Builder
	if o.UseLower {
		b.WriteString(lowercase)
	}
	if o.UseUpper {
		b.WriteString(uppercase)
	}
	if o.UseDigits {
		b.WriteString(digits)
	}
	if o.UseSymbols {
		b.WriteString(symbols)
	}
	return b.String()
}

// Generate returns a random password per the options, drawing each character
// uniformly from the pool using crypto/rand.
func Generate(o Options) (string, error) {
	if o.Length <= 0 {
		return "", errors.New("length must be positive")
	}
	pool := o.pool()
	if pool == "" {
		return "", errors.New("at least one character class must be enabled")
	}

	poolLen := big.NewInt(int64(len(pool)))
	out := make([]byte, o.Length)
	for i := range out {
		// rand.Int returns a uniform value in [0, poolLen) with NO modulo bias —
		// it internally rejects values that would skew the distribution.
		n, err := rand.Int(rand.Reader, poolLen)
		if err != nil {
			return "", err
		}
		out[i] = pool[n.Int64()]
	}
	return string(out), nil
}

// EntropyBits estimates the entropy of a password in bits, based on the size of
// the character pool it appears to draw from and its length:
//
//	bits = length * log2(poolSize)
//
// Each bit doubles the guessing effort. This is exact for randomly generated
// passwords; for human-chosen passwords it is an UPPER bound (real predictability
// like dictionary words makes them weaker than this suggests).
func EntropyBits(password string) float64 {
	if password == "" {
		return 0
	}
	poolSize := 0
	if strings.ContainsAny(password, lowercase) {
		poolSize += len(lowercase)
	}
	if strings.ContainsAny(password, uppercase) {
		poolSize += len(uppercase)
	}
	if strings.ContainsAny(password, digits) {
		poolSize += len(digits)
	}
	if strings.ContainsAny(password, symbols) {
		poolSize += len(symbols)
	}
	if poolSize == 0 {
		poolSize = 1 // fallback for exotic characters
	}
	return float64(len([]rune(password))) * math.Log2(float64(poolSize))
}

// Strength maps entropy bits to a human label.
func Strength(bits float64) string {
	switch {
	case bits < 40:
		return "weak"
	case bits < 60:
		return "fair"
	case bits < 80:
		return "strong"
	default:
		return "excellent"
	}
}
