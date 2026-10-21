package breach

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/db"
	"github.com/FayyazB106/account-manager/internal/store"
)

type fakeHIBP struct{ count *int }

func (f fakeHIBP) RoundTrip(r *http.Request) (*http.Response, error) {
	sum := sha1.Sum([]byte("hunter2-hunter2"))
	suffix := strings.ToUpper(hex.EncodeToString(sum[:]))[5:]
	body := "0000000000000000000000000000000000A:3\r\n"
	if *f.count > 0 {
		body += fmt.Sprintf("%s:%d\r\n", suffix, *f.count)
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}

// OnNew fires only for passwords that show up in more breaches than last time: the
// first time a breach is found, and again if the count grows — never for a breach
// that was already known.
func TestScanReportsOnlyNewBreaches(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	key, _ := crypto.RandomBytes(crypto.KeySize)
	env, err := crypto.EnvelopeEncrypt(key, []byte("hunter2-hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	accounts := store.NewAccountStore(d)
	login := "ops@example.com"
	if _, err := accounts.Create(context.Background(), &store.Account{Service: "Figma", Login: &login,
		PasswordWrappedDEK: env.WrappedDEK, PasswordCiphertext: env.Ciphertext}); err != nil {
		t.Fatal(err)
	}

	count := 0
	m := NewMonitor(accounts, store.NewSettingStore(d), func() []byte { return key })
	m.client = &http.Client{Transport: fakeHIBP{&count}}
	var got [][]store.BreachHit
	m.OnNew = func(_ context.Context, hits []store.BreachHit) { got = append(got, hits) }

	scan := func() {
		t.Helper()
		if _, _, err := m.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	scan() // clean password: nothing to report
	if len(got) != 0 {
		t.Fatal("reported a clean password")
	}
	count = 5
	scan() // newly breached
	if len(got) != 1 || got[0][0].Service != "Figma" || got[0][0].Login != login || got[0][0].Count != 5 {
		t.Fatalf("first detection: %+v", got)
	}
	scan() // same breach again: already known
	if len(got) != 1 {
		t.Fatalf("a known breach was reported again: %+v", got)
	}
	count = 9
	scan() // appears in more breaches now
	if len(got) != 2 || got[1][0].Count != 9 {
		t.Fatalf("a grown breach count should be reported: %+v", got)
	}
}
