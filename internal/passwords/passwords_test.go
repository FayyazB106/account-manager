package passwords

import (
	"strings"
	"testing"
)

func TestGenerateLengthAndCharset(t *testing.T) {
	o := Options{Length: 32, UseLower: true, UseDigits: true}
	pw, err := Generate(o)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len([]rune(pw)) != 32 {
		t.Fatalf("length: got %d want 32", len(pw))
	}
	// Every character must come from the enabled classes (lower + digits only).
	allowed := lowercase + digits
	for _, r := range pw {
		if !strings.ContainsRune(allowed, r) {
			t.Fatalf("unexpected character %q in %q", r, pw)
		}
	}
}

func TestGenerateRequiresAClass(t *testing.T) {
	_, err := Generate(Options{Length: 10}) // no classes enabled
	if err == nil {
		t.Fatal("expected error when no character class is enabled")
	}
}

func TestGenerateIsRandom(t *testing.T) {
	o := DefaultOptions()
	a, _ := Generate(o)
	b, _ := Generate(o)
	if a == b {
		t.Fatal("two generated passwords are identical (RNG not working?)")
	}
}

func TestEntropyIncreasesWithLengthAndPool(t *testing.T) {
	short := EntropyBits("aaaaaaaa")            // 8 chars, lowercase only
	long := EntropyBits("aaaaaaaaaaaaaaaa")     // 16 chars, lowercase only
	mixed := EntropyBits("aA1!aA1!")            // 8 chars, all four classes

	if !(long > short) {
		t.Fatalf("longer password should have more entropy: short=%.1f long=%.1f", short, long)
	}
	if !(mixed > short) {
		t.Fatalf("larger pool should raise entropy: lower=%.1f mixed=%.1f", short, mixed)
	}
}

func TestStrengthLabels(t *testing.T) {
	cases := map[float64]string{
		20:  "weak",
		50:  "fair",
		70:  "strong",
		130: "excellent",
	}
	for bits, want := range cases {
		if got := Strength(bits); got != want {
			t.Fatalf("Strength(%.0f): got %q want %q", bits, got, want)
		}
	}
}

// A generated 20-char all-class password should be very strong (>120 bits).
func TestGeneratedPasswordIsStrong(t *testing.T) {
	pw, _ := Generate(DefaultOptions())
	if bits := EntropyBits(pw); bits < 120 {
		t.Fatalf("expected a strong default password, got %.1f bits", bits)
	}
}
