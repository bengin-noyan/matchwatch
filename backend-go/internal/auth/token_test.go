package auth

import (
	"bytes"
	"testing"
)

func TestNewLinkToken(t *testing.T) {
	a, ha := NewLinkToken()
	b, _ := NewLinkToken()
	if len(a) != 43 || a == b || len(ha) != 32 || !bytes.Equal(ha, LinkTokenHash(a)) {
		t.Fatalf("token %q (%d) hash %x; second %q", a, len(a), ha, b)
	}
}
