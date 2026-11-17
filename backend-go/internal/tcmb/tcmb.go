// Package tcmb, Merkez Bankası'nın günlük gösterge kurlarını okur (kurlar/YYYYMM/DDMMYYYY.xml). Altyapımızdan yalnızca
// bir tarih çıkar; hiçbir müşteri verisi gönderilmez.
package tcmb

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

var code = regexp.MustCompile(`^[A-Z]{3}$`)

// Parse, bir bülteni okur; tarihini ve döviz alış kuru pozitif olan para birimlerini döner.
func Parse(r io.Reader) (time.Time, []domain.FXQuote, error) {
	var x struct {
		Tarih      string `xml:"Tarih,attr"`
		Currencies []struct {
			Code        string `xml:"CurrencyCode,attr"`
			Unit        string `xml:"Unit"`
			ForexBuying string `xml:"ForexBuying"`
		} `xml:"Currency"`
	}
	if err := xml.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&x); err != nil {
		return time.Time{}, nil, fmt.Errorf("tcmb: bad bulletin: %w", err)
	}
	day, err := time.Parse("02.01.2006", x.Tarih)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("tcmb: bad bulletin date %q", x.Tarih)
	}
	var out []domain.FXQuote
	for _, c := range x.Currencies {
		fb, err := decimal.NewFromString(strings.TrimSpace(c.ForexBuying))
		unit, uerr := decimal.NewFromString(strings.TrimSpace(c.Unit))
		if err != nil || uerr != nil || !fb.IsPositive() || !unit.IsPositive() || !unit.IsInteger() || !code.MatchString(c.Code) {
			continue // bazı para birimlerinin döviz kuru yayımlanmıyor
		}
		out = append(out, domain.FXQuote{Currency: c.Code, Unit: unit.IntPart(), ForexBuying: fb})
	}
	if len(out) == 0 {
		return time.Time{}, nil, errors.New("tcmb: bulletin has no rates")
	}
	return day, out, nil
}

type Client struct {
	base string
	hc   *http.Client
}

func New(baseURL string) *Client {
	return &Client{base: strings.TrimRight(baseURL, "/"), hc: &http.Client{Timeout: 10 * time.Second}}
}

// Day, o gün yayımlanan bülteni getirir. published=false (hatasız) TCMB'nin o gün bülten yayımlamadığını gösterir
// (hafta sonu, tatil: HTTP 404). Başka her sorun hatadır ve "bülten yok" diye önbelleğe alınmamalıdır.
func (c *Client) Day(ctx context.Context, day time.Time) ([]domain.FXQuote, bool, error) {
	url := fmt.Sprintf("%s/%s/%s.xml", c.base, day.Format("200601"), day.Format("02012006"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("tcmb: %s unavailable", day.Format("2006-01-02"))
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("tcmb: %s: HTTP %d", day.Format("2006-01-02"), resp.StatusCode)
	}
	got, rates, err := Parse(resp.Body)
	if err != nil {
		return nil, false, err
	}
	if !got.Equal(time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)) {
		return nil, false, fmt.Errorf("tcmb: asked %s, bulletin is %s", day.Format("2006-01-02"), got.Format("2006-01-02"))
	}
	return rates, true, nil
}
