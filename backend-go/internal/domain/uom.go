package domain

import "strings"

var uomMap = map[string]string{"C62": "ADET", "KGM": "KG", "LTR": "LT", "MTR": "M", "NIU": "ADET"}

func NormalizeUOM(code string) string {
	c := strings.ToUpper(strings.TrimSpace(code))
	if v, ok := uomMap[c]; ok {
		return v
	}
	return c
}
