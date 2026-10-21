package api

import "net/http"

// allowedOrigin is the web client's dev origin. CORS is deliberately strict: we
// name ONE origin rather than "*", so only our app may call the API from a
// browser. In production this becomes the deployed web app's URL.
const allowedOrigin = "http://localhost:5173"

// withCORS wraps a handler to answer browser CORS preflight requests and add the
// required response headers. Only the specific origin, methods, and headers we
// use are permitted.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-App-Language")

		// A CORS "preflight" OPTIONS request just asks permission; answer it here.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
