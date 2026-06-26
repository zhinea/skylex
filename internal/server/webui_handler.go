package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the embedded Vite SPA under the /panel/ prefix.
//
// Real asset requests (existing files in the build) are served directly with
// the default content-type detection and caching headers from http.FileServer.
// Any other path under /panel/ falls back to index.html so client-side routing
// (react-router BrowserRouter) can take over.
func spaHandler(buildFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(buildFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip the /panel prefix to map onto the embedded FS layout.
		reqPath := strings.TrimPrefix(r.URL.Path, "/panel")
		reqPath = strings.TrimPrefix(reqPath, "/")
		if reqPath == "" {
			serveIndex(w, r, buildFS)
			return
		}

		if f, err := buildFS.Open(reqPath); err == nil {
			f.Close()
			// Long-cache hashed assets; index.html is handled separately.
			if strings.HasPrefix(reqPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.StripPrefix("/panel/", fileServer).ServeHTTP(w, r)
			return
		}

		// Unknown path with a file extension -> genuine 404 (missing asset).
		if path.Ext(reqPath) != "" {
			http.NotFound(w, r)
			return
		}

		// Client-side route -> serve the SPA shell.
		serveIndex(w, r, buildFS)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, buildFS fs.FS) {
	data, err := fs.ReadFile(buildFS, "index.html")
	if err != nil {
		http.Error(w, "panel not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}
