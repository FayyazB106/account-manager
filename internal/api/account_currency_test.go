package api

import "testing"

func TestNormalizeCurrency(t *testing.T) {
	for in, want := range map[string]string{"": "USD", " sar ": "SAR", "eur": "EUR", "USD": "USD"} {
		if got, ok := normalizeCurrency(in); !ok || got != want {
			t.Errorf("normalizeCurrency(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"US", "DOLLAR", "U$D", "12A"} {
		if _, ok := normalizeCurrency(bad); ok {
			t.Errorf("normalizeCurrency(%q) should be rejected", bad)
		}
	}
}
