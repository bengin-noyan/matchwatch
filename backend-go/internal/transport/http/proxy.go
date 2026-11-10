package transporthttp

import (
	"net"
	"net/http"
	"strings"
)

// BehindProxy takes the client address from the right-most X-Forwarded-For entry, the one our own reverse proxy
// (Caddy) wrote; entries left of it come from the client and can be forged. Use it only when the API is reachable
// solely through that proxy (TRUST_PROXY=true).
func BehindProxy(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xff := r.Header.Values("X-Forwarded-For")
		if len(xff) > 0 {
			last := xff[len(xff)-1]
			ip := strings.TrimSpace(last[strings.LastIndex(last, ",")+1:])
			if net.ParseIP(ip) != nil {
				r2 := r.Clone(r.Context())
				r2.RemoteAddr = net.JoinHostPort(ip, "0")
				r = r2
			}
		}
		h.ServeHTTP(w, r)
	})
}
