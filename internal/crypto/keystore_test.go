package crypto

import (
	"bytes"
	"testing"
)

func TestSealUnsealRoundTrip(t *testing.T) {
	master, _ := RandomBytes(KeySize)
	sk, err := SealKey(master, "correct horse battery staple")
	if err != nil {
		t.Fatalf("SealKey: %v", err)
	}
	// The sealed blob must not contain the raw key.
	if bytes.Contains([]byte(sk.Blob), master) {
		t.Fatal("sealed blob leaks the master key")
	}
	got, err := UnsealKey(sk, "correct horse battery staple")
	if err != nil {
		t.Fatalf("UnsealKey: %v", err)
	}
	if !bytes.Equal(got, master) {
		t.Fatal("round-trip mismatch")
	}
}

func TestUnsealWrongPassphraseFails(t *testing.T) {
	master, _ := RandomBytes(KeySize)
	sk, _ := SealKey(master, "right-passphrase")
	if _, err := UnsealKey(sk, "wrong-passphrase"); err == nil {
		t.Fatal("expected wrong passphrase to fail")
	}
}
