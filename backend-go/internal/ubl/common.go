// Package ubl parses UBL-TR 1.2 documents. Elements are matched by namespace
// URI (the struct tags), never by prefix.
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

// Namespace-qualified tag prefixes can't be constants in struct tags, so the
// URIs are spelled out: cbc = ...CommonBasicComponents-2, cac = ...CommonAggregateComponents-2.

// ValidationError reports a missing or invalid field in a UBL document.
type ValidationError struct{ Field, Reason string }

func (e *ValidationError) Error() string { return "ubl: " + e.Field + ": " + e.Reason }

// Party is a trading party reduced to what matching needs.
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

// party converts and validates; field names the party in errors (e.g. "Supplier").
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

// parseDecimal returns zero for empty input (optional elements).
func parseDecimal(field, s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, &ValidationError{field, fmt.Sprintf("invalid number %q", s)}
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

// parseOptDate returns nil for empty input.
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

// decodeRoot decodes the document into v if its root element is {ns}local, else returns notMatch.
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
