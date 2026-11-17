package domain

import (
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var trFold = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u", "â", "a", "î", "i", "û", "u")

// NameKey, bir kalem adını eşleşme araması ve embedding için tek biçime getirir: Türkçe küçük harf, aksanlar sadeleşir,
// harf ve rakam dışındaki her şey boşluk olur, fazla boşluklar silinir. "AYÇİÇEK YAĞI 5LT/TNK" -> "aycicek yagi 5lt
// tnk".
func NameKey(s string) string {
	s = trFold.Replace(strings.ToLowerSpecial(unicode.TurkishCase, s))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// AliasKey, bir insanın onayladığı fatura kalem adıdır (tedarikçiye göre); bir sözleşme kalemine karşılık gelir (D16).
type AliasKey struct{ NameKey, UOM string }

// Suggestion, eşleşmeyen bir fatura satırı için embedding'le sıralanmış bir sözleşme kalemi adayıdır. Kendi başına asla
// eşleşme sayılmaz.
type Suggestion struct {
	ContractItemID uuid.UUID
	ItemName       string
	Similarity     decimal.Decimal
}
