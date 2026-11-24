package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AuditTenants, girişi olan bütün tenant'ları döner (audit_tenants SECURITY DEFINER).
func (s *Store) AuditTenants(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT tenant_id FROM audit_tenants()`)
	if err != nil {
		return nil, wrap("audit tenants", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return ids, wrap("audit tenants", err)
}

// StuckInvoices, 5 dakikadan uzun süredir PENDING kalan faturaları en eskiden başlayarak döner. Daha yenilerin denetimi
// hâlâ yükleme isteğinde sürüyor olabilir.
func (tx *Tx) StuckInvoices(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM invoices WHERE tenant_id=$1 AND audit_status='PENDING'
		AND created_at < now() - interval '5 minutes' ORDER BY created_at, id`, tx.tenantID)
	if err != nil {
		return nil, wrap("stuck invoices", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return ids, wrap("stuck invoices", err)
}
