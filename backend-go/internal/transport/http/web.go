package transporthttp

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WithWeb serves the built SPA (web/dist) from dir on the same origin as the API, so no CORS is needed and the
// SameSite=Strict session cookie works. /api/ and /healthz go to api; unknown paths get index.html (client routing).
func WithWeb(api http.Handler, dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, 405, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
			"object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		clean := path.Clean("/" + r.URL.Path)
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(clean))); err == nil && !st.IsDir() {
			if strings.HasPrefix(clean, "/assets/") { // Vite fingerprints these file names
				h.Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			if strings.HasSuffix(clean, ".mjs") { // the OS mime table (Windows registry, /etc/mime.types) may say text/plain
				h.Set("Content-Type", "text/javascript; charset=utf-8")
			}
			files.ServeHTTP(w, r)
			return
		}
		h.Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
