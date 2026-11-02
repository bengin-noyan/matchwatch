package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestToMinor(t *testing.T) {
	cases := map[string]int64{"1.005": 101, "-1.005": -101, "50.00": 5000, "3698.630137": 369863}
	for in, want := range cases {
		if got := ToMinor(decimal.RequireFromString(in)); got != want {
			t.Errorf("ToMinor(%s)=%d want %d", in, got, want)
		}
	}
}

func TestFromMinor(t *testing.T) {
	if got := FromMinor(369863).String(); got != "3698.63" {
		t.Errorf("got %s", got)
	}
}

func TestNetUnitPrice(t *testing.T) {
	l := InvoiceLine{Qty: decimal.NewFromInt(1000), LineExtension: decimal.NewFromInt(1300)}
	if got := l.NetUnitPrice().String(); got != "1.3" {
		t.Errorf("got %s", got)
	}
}
