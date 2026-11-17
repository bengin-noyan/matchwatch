package domain

import "github.com/shopspring/decimal"

// ToMinor, tutarı kuruşa çevirir; Round(2) yarımları sıfırdan uzağa yuvarlar.
func ToMinor(d decimal.Decimal) int64 { return d.Round(2).Shift(2).IntPart() }

func FromMinor(m int64) decimal.Decimal { return decimal.New(m, -2) }
