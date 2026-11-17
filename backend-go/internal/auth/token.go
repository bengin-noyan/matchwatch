package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// NewLinkToken returns a random one-time token for an invite or reset link and the hash the database keeps.
func NewLinkToken() (token string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b) // never fails (crypto/rand panics instead)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, LinkTokenHash(token)
}

// LinkTokenHash is the SHA-256 the database stores; a 256-bit random token needs no slow hash.
func LinkTokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
