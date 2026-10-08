package api

import (
	"net/http"
	"strconv"

	"github.com/FayyazB106/account-manager/internal/passwords"
)

// generatePassword handles GET /generate. Query params (all optional):
//
//	length  = integer (default 20, max 128)
//	lower   = true|false (default true)
//	upper   = true|false (default true)
//	digits  = true|false (default true)
//	symbols = true|false (default true)
//
// Returns the generated password plus its entropy and a strength label.
func (s *Server) generatePassword(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	opts := passwords.DefaultOptions()
	opts.UseLower = boolParam(q.Get("lower"), true)
	opts.UseUpper = boolParam(q.Get("upper"), true)
	opts.UseDigits = boolParam(q.Get("digits"), true)
	opts.UseSymbols = boolParam(q.Get("symbols"), true)

	if raw := q.Get("length"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 128 {
			writeError(w, http.StatusBadRequest, "length must be an integer between 1 and 128")
			return
		}
		opts.Length = n
	}

	pw, err := passwords.Generate(opts)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	bits := passwords.EntropyBits(pw)
	writeJSON(w, http.StatusOK, map[string]any{
		"password":     pw,
		"entropy_bits": round1(bits),
		"strength":     passwords.Strength(bits),
	})
}

// boolParam parses an optional "true"/"false" query value, falling back to def.
func boolParam(raw string, def bool) bool {
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return b
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
