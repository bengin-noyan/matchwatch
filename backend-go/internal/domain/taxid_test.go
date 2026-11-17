package domain

import "testing"

func TestValidTaxID(t *testing.T) {
	for s, want := range map[string]bool{
		"1234567890":  true,  // VKN
		"1234567808":  true,  // VKN
		"1234567891":  false, // kontrol hanesi yanlış
		"9876543210":  false, // geliştirme verisindeki VKN gerçek değil
		"10000000146": true,  // TCKN
		"10000000147": false,
		"01234567890": false, // TCKN asla 0 ile başlamaz
		"12345a7890":  false,
		"":            false,
		"123456789":   false,
	} {
		if got := ValidTaxID(s); got != want {
			t.Errorf("ValidTaxID(%q) = %v, want %v", s, got, want)
		}
	}
}
