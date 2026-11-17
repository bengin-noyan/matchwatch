package anomaly

import "github.com/bengin-noyan/matchwatch/backend-go/internal/domain"

// Status: bir tane bile CRITICAL varsa RED, yoksa WARNING varsa YELLOW, hiçbiri yoksa GREEN. INFO bulguları durumu
// etkilemez.
func Status(fs []domain.Finding) domain.AuditStatus {
	st := domain.AuditGreen
	for _, f := range fs {
		switch f.Severity {
		case domain.SeverityCritical:
			return domain.AuditRed
		case domain.SeverityWarning:
			st = domain.AuditYellow
		}
	}
	return st
}
