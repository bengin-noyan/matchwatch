package domain

import "github.com/shopspring/decimal"

// ToMinor converts to kuruş; Round(2) is half-away-from-zero.
func ToMinor(d decimal.Decimal) int64 { return d.Round(2).Shift(2).IntPart() }

func FromMinor(m int64) decimal.Decimal { return decimal.New(m, -2) }
