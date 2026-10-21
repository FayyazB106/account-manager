package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/FayyazB106/account-manager/internal/auth"
)

// ctxKey is an unexported type for context keys, so no other package can collide
// with ours.
type ctxKey string

const adminIDKey ctxKey = "adminID"

// requireAuth wraps a handler so it only runs for requests carrying a valid
// session token. The token is read from the "Authorization: Bearer <token>"
// header, verified, and the admin's ID is stashed in the request context for the
// inner handler to use.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		prefix := "Bearer "
		if !strings.HasPrefix(header, prefix) {
			writeError(w, http.StatusUnauthorized, "missing or malformed authorization header")
			return
		}
		tokenString := strings.TrimPrefix(header, prefix)

		adminID, err := auth.ParseToken(tokenString, s.JWTSecret)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		// Attribute this request to the admin for the audit middleware (which
		// created the holder before routing; a no-op if the route isn't audited).
		if actor, ok := auditActorFrom(r.Context()); ok {
			actor.adminID = adminID
		}

		// Learn which language the app is showing this admin (for the emails sent to them).
		s.noteLanguage(r, adminID)

		// Pass the authenticated admin's ID down to the handler via context.
		ctx := context.WithValue(r.Context(), adminIDKey, adminID)
		next(w, r.WithContext(ctx))
	}
}

// adminIDFrom retrieves the authenticated admin ID placed by requireAuth.
func adminIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(adminIDKey).(string)
	return id, ok
}

// noteLanguage records the language the app is being shown in, which the web app sends
// on every request as X-App-Language ("en" or "ar"), as the admin's language — so the
// emails sent to them (scheduled ones run with no browser) are written in it, and
// "Send Now" matches the screen they are looking at. It runs before the handler, so a
// request's own language is already in place when the handler reads it. A small
// in-memory map keeps this to one database write per change, not one per request.
func (s *Server) noteLanguage(r *http.Request, adminID string) {
	lang := r.Header.Get("X-App-Language")
	if lang != "en" && lang != "ar" {
		return // absent or unknown: leave the stored language alone
	}
	s.langMu.Lock()
	seen := s.langSeen[adminID]
	s.langMu.Unlock()
	if seen == lang {
		return
	}
	if err := s.Admins.SetLanguage(r.Context(), adminID, lang); err != nil {
		logErr("noteLanguage", err)
		return
	}
	s.langMu.Lock()
	if s.langSeen == nil {
		s.langSeen = map[string]string{}
	}
	s.langSeen[adminID] = lang
	s.langMu.Unlock()
}
