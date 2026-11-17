package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// NewLinkToken, davet ya da şifre sıfırlama bağlantısı için rastgele, tek kullanımlık bir token ve veritabanında
// saklanacak hash'ini üretir.
func NewLinkToken() (token string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b) // hata vermez (crypto/rand başarısız olursa panic eder)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, LinkTokenHash(token)
}

// LinkTokenHash, veritabanında saklanan SHA-256 değeridir. 256 bitlik rastgele bir token'a yavaş bir hash gerekmez.
func LinkTokenHash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
