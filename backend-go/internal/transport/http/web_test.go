package transporthttp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithWeb(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644)
	os.WriteFile(filepath.Join(dir, "assets", "app-1a2b.js"), []byte("js"), 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("secret"), 0o644)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("api")) })
	h := WithWeb(api, dir)
	get := func(method, p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, p, nil))
		return rec
	}
	if rec := get("GET", "/api/v1/invoices"); rec.Body.String() != "api" {
		t.Errorf("api not routed: %q", rec.Body)
	}
	if rec := get("POST", "/api/v1/auth/login"); rec.Body.String() != "api" {
		t.Errorf("api POST not routed: %q", rec.Body)
	}
	if rec := get("GET", "/assets/app-1a2b.js"); rec.Body.String() != "js" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %q %v", rec.Body, rec.Header())
	}
	if rec := get("GET", "/../secret.txt"); strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("path traversal served %q", rec.Body)
	}
	for _, p := range []string{"/faturalar/123", "/"} {
		rec := get("GET", p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "app") || strings.Contains(rec.Body.String(), "secret") ||
			!strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Errorf("%s: %d %q", p, rec.Code, rec.Body)
		}
	}
	if rec := get("POST", "/faturalar"); rec.Code != 405 {
		t.Errorf("POST to SPA: %d", rec.Code)
	}
}
