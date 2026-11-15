package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FayyazB106/account-manager/internal/db"
)

func strp(s string) *string { return &s }

// Seats must be people from the Users list: matched ignoring case (and stored with the
// person's exact name and Arabic name), unknown names are refused, blank seats pass, and
// seats an account already had are grandfathered.
func TestValidateSeats(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	people := NewPersonStore(d)
	ctx := context.Background()
	if _, err := people.Create(ctx, "Mark", strp("مارك")); err != nil {
		t.Fatal(err)
	}
	if _, err := people.Create(ctx, "Ahmed", nil); err != nil {
		t.Fatal(err)
	}

	names, ar, bad, err := people.ValidateSeats(ctx, []string{" mark ", "", "AHMED"}, nil, nil)
	if err != nil || len(bad) != 0 {
		t.Fatalf("known people should pass: bad=%v err=%v", bad, err)
	}
	if names[0] != "Mark" || ar[0] == nil || *ar[0] != "مارك" || names[1] != "" || names[2] != "Ahmed" {
		t.Errorf("seats should be stored with the person's exact name: %q %v", names, ar)
	}

	_, _, bad, err = people.ValidateSeats(ctx, []string{"Mark", "user", "hello"}, nil, nil)
	if err != nil || len(bad) != 2 || bad[0] != "user" || bad[1] != "hello" {
		t.Fatalf("names outside the Users list must be refused: bad=%v err=%v", bad, err)
	}

	// An account that already has a free-text "user" seat can keep it.
	names, _, bad, err = people.ValidateSeats(ctx, []string{"Mark", "user"}, nil, []string{"User"})
	if err != nil || len(bad) != 0 || names[1] != "user" {
		t.Fatalf("an existing seat is grandfathered: names=%v bad=%v err=%v", names, bad, err)
	}
}
