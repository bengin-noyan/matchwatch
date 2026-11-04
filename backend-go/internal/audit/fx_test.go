package audit

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

func TestCrossRates(t *testing.T) {
	d := decimal.RequireFromString
	quotes := []domain.FXQuote{{Currency: "USD", Unit: 1, ForexBuying: d("40")}, {Currency: "EUR", Unit: 1, ForexBuying: d("44")},
		{Currency: "JPY", Unit: 100, ForexBuying: d("26")}}
	day := time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC)
	got := crossRates("EUR", map[string]bool{"USD": true, "TRY": true, "JPY": true, "GBP": true}, quotes, day)
	if got["USD"].Rate.String() != "1.1" || got["TRY"].Rate.String() != "44" || got["JPY"].Rate.String() != "169.2307692307692308" {
		t.Fatalf("%+v", got)
	}
	if _, ok := got["GBP"]; ok || !got["USD"].TCMBDate.Equal(day) || got["USD"].Source != "EUR" || got["USD"].Target != "USD" {
		t.Fatalf("%+v", got)
	}
	if got := crossRates("TRY", map[string]bool{"USD": true}, quotes, day); got["USD"].Rate.String() != "0.025" {
		t.Fatalf("TRY->USD %+v", got)
	}
	if got := crossRates("CHF", map[string]bool{"USD": true}, quotes, day); len(got) != 0 {
		t.Fatalf("unquoted source %+v", got)
	}
}
