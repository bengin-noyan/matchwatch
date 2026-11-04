package tcmb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/03112026.xml") // real TCMB bulletin of 2026-11-03
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseRealBulletin(t *testing.T) {
	day, rates, err := Parse(strings.NewReader(string(fixture(t))))
	if err != nil {
		t.Fatal(err)
	}
	if day.Format("2006-01-02") != "2026-11-03" || len(rates) != 22 {
		t.Fatalf("day=%s n=%d", day, len(rates))
	}
	by := map[string]Rate{}
	for _, r := range rates {
		by[r.Currency] = r
	}
	if r := by["USD"]; r.Unit != 1 || r.ForexBuying.String() != "49.9094" {
		t.Errorf("USD %+v", r)
	}
	if r := by["JPY"]; r.Unit != 100 || r.ForexBuying.String() != "31.5049" {
		t.Errorf("JPY %+v", r)
	}
}

func TestParseRejects(t *testing.T) {
	for name, body := range map[string]string{
		"html":     `<html><title>Sayfa Goruntulenemedi</title></html>`,
		"bad date": `<Tarih_Date Tarih="x"><Currency CurrencyCode="USD"><Unit>1</Unit><ForexBuying>1</ForexBuying></Currency></Tarih_Date>`,
		"no rates": `<Tarih_Date Tarih="03.11.2026"><Currency CurrencyCode="USD"><Unit>1</Unit><ForexBuying></ForexBuying></Currency></Tarih_Date>`,
		"garbage":  `{`,
	} {
		if _, _, err := Parse(strings.NewReader(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/kurlar/202611/03112026.xml":
			w.Write(fixture(t))
		case "/kurlar/202611/04112026.xml": // a server that answers with the wrong day
			w.Write(fixture(t))
		case "/kurlar/202611/05112026.xml":
			w.WriteHeader(500)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := New(srv.URL + "/kurlar/")
	ctx := context.Background()
	d := func(day int) time.Time { return time.Date(2026, 11, day, 0, 0, 0, 0, time.UTC) }

	rates, ok, err := c.Day(ctx, d(3))
	if err != nil || !ok || len(rates) != 22 {
		t.Fatalf("weekday: ok=%v n=%d err=%v", ok, len(rates), err)
	}
	if _, ok, err := c.Day(ctx, d(1)); err != nil || ok {
		t.Fatalf("weekend: ok=%v err=%v", ok, err)
	}
	if _, _, err := c.Day(ctx, d(4)); err == nil {
		t.Fatal("wrong-day bulletin accepted")
	}
	if _, _, err := c.Day(ctx, d(5)); err == nil {
		t.Fatal("HTTP 500 treated as no bulletin")
	}
}
