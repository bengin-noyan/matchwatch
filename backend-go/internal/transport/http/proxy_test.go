package transporthttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBehindProxyUsesRightmostForwardedFor(t *testing.T) {
	var got string
	h := BehindProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
	for _, c := range []struct {
		xff  []string
		want string
	}{
		{nil, "10.0.0.9:5555"},
		{[]string{"203.0.113.7"}, "203.0.113.7:0"},
		{[]string{"6.6.6.6, 203.0.113.7"}, "203.0.113.7:0"},   // forged left entry is ignored
		{[]string{"6.6.6.6", "203.0.113.7"}, "203.0.113.7:0"}, // two header lines
		{[]string{"2001:db8::1"}, "[2001:db8::1]:0"},
		{[]string{"not-an-ip"}, "10.0.0.9:5555"},
	} {
		r := httptest.NewRequest("POST", "/api/v1/auth/signup", nil)
		r.RemoteAddr = "10.0.0.9:5555"
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if got != c.want {
			t.Errorf("xff %q: got %q, want %q", c.xff, got, c.want)
		}
	}
}
