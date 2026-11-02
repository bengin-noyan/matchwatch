package domain

import "testing"

func TestNormalizeUOM(t *testing.T) {
	cases := map[string]string{"C62": "ADET", " kgm ": "KG", "HUR": "HUR", "NIU": "ADET", "LTR": "LT", "MTR": "M"}
	for in, want := range cases {
		if got := NormalizeUOM(in); got != want {
			t.Errorf("NormalizeUOM(%q)=%q want %q", in, got, want)
		}
	}
}
