package domain

import "testing"

func TestValidTaxID(t *testing.T) {
	for s, want := range map[string]bool{
		"1234567890":  true,  // VKN
		"1234567808":  true,  // VKN
		"1234567891":  false, // wrong check digit
		"9876543210":  false, // the dev seed's VKN is not a real one
		"10000000146": true,  // TCKN
		"10000000147": false,
		"01234567890": false, // a TCKN never starts with 0
		"12345a7890":  false,
		"":            false,
		"123456789":   false,
	} {
		if got := ValidTaxID(s); got != want {
			t.Errorf("ValidTaxID(%q) = %v, want %v", s, got, want)
		}
	}
}
