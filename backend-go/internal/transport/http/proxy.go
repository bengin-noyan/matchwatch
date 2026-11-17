package transporthttp

import (
	"net"
	"net/http"
	"strings"
)

// BehindProxy, istemci adresini X-Forwarded-For'un en sağdaki girdisinden alır; o girdiyi bizim reverse proxy'miz
// (Caddy) yazar. Solundaki girdiler istemciden gelir ve sahte olabilir. Bunu yalnızca API'ye sadece o proxy üzerinden
// ulaşılabiliyorsa kullan (TRUST_PROXY=true).
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
