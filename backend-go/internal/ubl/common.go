// Package ubl, UBL-TR 1.2 belgelerini okur. Elemanlar ön ekle değil, namespace URI'siyle (struct etiketleri) eşleşir.
package ubl

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Namespace'li etiket ön ekleri struct etiketlerinde sabit olarak yazılamıyor, o yüzden URI'ler açık yazıldı: cbc =
// ...CommonBasicComponents-2, cac = ...CommonAggregateComponents-2.

// ValidationError, bir UBL belgesinde eksik ya da geçersiz bir alanı bildirir.
type ValidationError struct{ Field, Reason string }

func (e *ValidationError) Error() string { return "ubl: " + e.Field + ": " + e.Reason }

// Party, bir ticari tarafın eşleştirme için gereken kısmıdır.
type Party struct{ VKN, Title string }

type xmlParty struct {
	IDs []struct {
		ID struct {
			Scheme string `xml:"schemeID,attr"`
			Value  string `xml:",chardata"`
		} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 ID"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 PartyIdentification"`
	Name struct {
		Value string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 Name"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 PartyName"`
	Person struct {
		First  string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 FirstName"`
		Family string `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2 FamilyName"`
	} `xml:"urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2 Person"`
}

var vknRe = regexp.MustCompile(`^(\d{10}|\d{11})$`)

// party çevirir ve doğrular; field, hata mesajlarında tarafın adıdır (ör. "Supplier").
func (p xmlParty) party(field string) (Party, error) {
	var out Party
	for _, id := range p.IDs {
		if s := strings.ToUpper(strings.TrimSpace(id.ID.Scheme)); s == "VKN" || s == "TCKN" {
			out.VKN = strings.TrimSpace(id.ID.Value)
			break
		}
	}
	out.Title = strings.TrimSpace(p.Name.Value)
	if out.Title == "" {
		out.Title = strings.TrimSpace(p.Person.First + " " + p.Person.Family)
	}
	if !vknRe.MatchString(out.VKN) {
		return out, &ValidationError{field + ".VKN", fmt.Sprintf("must be 10 or 11 digits, got %q", out.VKN)}
	}
	return out, nil
}

var (
	maxAmount  = decimal.New(1, 15) // tutarlar ve miktarlar: kuruşu int64 ve NUMERIC(19,4) içinde tutar
	maxPrice   = decimal.New(1, 13) // NUMERIC(19,6) en fazla 13 tam hane alır
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
)

func checkCurrency(field, s string) error {
	if !currencyRe.MatchString(s) {
		return &ValidationError{field, fmt.Sprintf("must be three uppercase letters, got %q", s)}
	}
	return nil
}

// parseDecimal boş girdi için sıfır döner (isteğe bağlı elemanlar); mutlak değeri 1e15 ya da üstü olan reddedilir.
func parseDecimal(field, s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, &ValidationError{field, fmt.Sprintf("invalid number %q", s)}
	}
	// önce üsse bakıyoruz: 1e999999999 ile karşılaştırma gigabaytlarca bellek ayırırdı
	if d.Exponent() < -12 || d.Exponent() > 12 || d.Abs().GreaterThanOrEqual(maxAmount) {
		return decimal.Zero, &ValidationError{field, fmt.Sprintf("out of range %q", s)}
	}
	return d, nil
}

func parseDate(field, s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, &ValidationError{field, fmt.Sprintf("invalid date %q, want YYYY-MM-DD", s)}
	}
	return t, nil
}

// parseOptDate boş girdi için nil döner.
func parseOptDate(field, s string) (*time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	t, err := parseDate(field, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// decodeRoot, kök eleman {ns}local ise belgeyi v'ye çözer, değilse notMatch döner.
func decodeRoot(r io.Reader, ns, local string, notMatch error, v any) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if s, ok := tok.(xml.StartElement); ok {
			if s.Name.Space != ns || s.Name.Local != local {
				return notMatch
			}
			return dec.DecodeElement(v, &s)
		}
	}
}
