package domain

import (
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var trFold = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u", "â", "a", "î", "i", "û", "u")

// NameKey normalizes an item name for alias lookup and embedding: Turkish lowercase, diacritics folded,
// anything but letters/digits becomes a space, whitespace collapsed. "AYÇİÇEK YAĞI 5LT/TNK" -> "aycicek yagi 5lt tnk".
func NameKey(s string) string {
	s = trFold.Replace(strings.ToLowerSpecial(unicode.TurkishCase, s))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// AliasKey is a human-confirmed invoice item name (per vendor) that maps to a contract item (D16).
type AliasKey struct{ NameKey, UOM string }

// Suggestion is an embedding-ranked contract item candidate for an unmatched invoice line; never a match by itself.
type Suggestion struct {
	ContractItemID uuid.UUID
	ItemName       string
	Similarity     decimal.Decimal
}
