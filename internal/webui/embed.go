// Package webui embeds the built React frontend and serves it as a single-page
// app, so the whole product ships as one self-contained binary.
//
// The build output is emitted here by Vite (build.outDir = ../internal/webui/dist).
// A committed placeholder index.html keeps `go build` working before the first
// frontend build; a real `npm run build` overwrites the dist folder.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded SPA: real files (JS, CSS, assets) are served
// directly; any other path falls back to index.html so client-side routing works
// on deep links and refreshes. Mount this as the catch-all, with the API under
// its own prefix (/api).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed guarantees dist exists; a failure is a build-time bug
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		// If the requested path isn't a real embedded file, it's a client-side
		// route — serve the SPA entry point instead of 404ing.
		if _, statErr := fs.Stat(sub, p); statErr != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
