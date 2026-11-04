package domain

import "testing"

func TestNameKey(t *testing.T) {
	for in, want := range map[string]string{
		"AYÇİÇEK YAĞI 5LT/TNK":   "aycicek yagi 5lt tnk",
		"AYCICEK YAGI 5LT TNK":   "aycicek yagi 5lt tnk",
		"  Işık  ":               "isik",
		"M8x20 paslanmaz civata": "m8x20 paslanmaz civata",
		"Şeker (Toz), 50-kg":     "seker toz 50 kg",
		"Ürün Ömrü":              "urun omru",
		"---":                    "",
	} {
		if got := NameKey(in); got != want {
			t.Errorf("NameKey(%q) = %q, want %q", in, got, want)
		}
	}
}
