package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityWarning  Severity = "WARNING"
	SeverityInfo     Severity = "INFO"
)

type RuleCode string

const (
	RulePriceMismatch      RuleCode = "PRICE_MISMATCH"
	RuleQtyExceedsDelivery RuleCode = "QTY_EXCEEDS_DELIVERY"
	RuleUncontractedItem   RuleCode = "UNCONTRACTED_ITEM"
	RuleMaturityViolation  RuleCode = "MATURITY_VIOLATION"
	RuleDuplicateSuspect   RuleCode = "DUPLICATE_SUSPECT"
	RuleWithholdingError   RuleCode = "WITHHOLDING_ERROR"
	RulePriceCreep         RuleCode = "PRICE_CREEP"
	RuleFXUnverified       RuleCode = "FX_UNVERIFIED"
	RuleUOMMismatch        RuleCode = "UOM_MISMATCH"
	RuleFXRateDeviation    RuleCode = "FX_RATE_DEVIATION"
)

type AuditStatus string

const (
	AuditGreen  AuditStatus = "GREEN"
	AuditYellow AuditStatus = "YELLOW"
	AuditRed    AuditStatus = "RED"
)

type Resolution string

const (
	ResolutionOpen              Resolution = "OPEN"
	ResolutionDisputed          Resolution = "DISPUTED"
	ResolutionApprovedException Resolution = "APPROVED_EXCEPTION"
)

type ExchangeRate struct {
	Source, Target string
	Rate           decimal.Decimal
	TCMBDate       time.Time // sıfır değilse kur faturadan değil, o günün TCMB bülteninden geldi (D17)
}

// FXQuote, TCMB bülteninin bir satırıdır: Unit birim Currency, ForexBuying TRY eder.
type FXQuote struct {
	Currency    string
	Unit        int64
	ForexBuying decimal.Decimal
}

type Invoice struct {
	ID, VendorID    uuid.UUID
	Number          string
	Currency        string
	IssueDate       time.Time
	DueDate         *time.Time
	ExchangeRate    *ExchangeRate
	PayableMinor    int64
	HasDespatchRefs bool
	Lines           []InvoiceLine
}

type InvoiceLine struct {
	ID                                                      uuid.UUID
	LineID, ItemCode, Barcode, Name, UOM                    string
	Qty, UnitPrice, AllowanceAmount, LineExtension, VATRate decimal.Decimal
	WithholdingPercent, WithholdingAmount                   decimal.Decimal
	WithholdingCode                                         string
}

// NetUnitPrice, Qty'nin sıfırdan büyük olduğunu varsayar (parser bunu garanti ediyor).
func (l InvoiceLine) NetUnitPrice() decimal.Decimal { return l.LineExtension.Div(l.Qty) }

type Contract struct {
	ID              uuid.UUID
	Number          string
	PaymentTermDays int
	Currency        string
	QtyTolerancePct decimal.Decimal
	// ödeme vadesinin sözleşmede yazdığı yer; sayfa 0 bilinmiyor demek
	PaymentTermSourcePage int
	PaymentTermClauseText string
}

type ContractItem struct {
	ID                                                                        uuid.UUID
	ItemCode, Barcode, Name, UOM, Currency, WithholdingCode, SourceClauseText string
	UnitPrice                                                                 decimal.Decimal
	SourcePage                                                                int
	RequiresDelivery                                                          bool
}

type DespatchCandidate struct {
	LineID                 uuid.UUID
	ItemCode, Barcode, UOM string
	Available              decimal.Decimal
	ShipmentDate           time.Time
}

type Allocation struct {
	InvoiceLineID, DespatchLineID uuid.UUID
	Qty                           decimal.Decimal
}

type PricePoint struct {
	IssueDate    time.Time
	NetUnitPrice decimal.Decimal
}

type DuplicateCandidate struct {
	InvoiceID    uuid.UUID
	Number       string
	IssueDate    time.Time
	PayableMinor int64
	LineNames    []string
}

type Finding struct {
	InvoiceLineID  *uuid.UUID
	Severity       Severity
	RuleCode       RuleCode
	Expected       string
	Actual         string
	ImpactMinor    int64
	ImpactCurrency string
	Evidence       map[string]any
}
