package repository

import (
	"context"
	"time"

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

// MarkReaudit, faturaları id sırasıyla kilitler ve yeniden denetlenecek diye işaretler. Değişiklikle aynı transaction'da
// çağrılır; işareti başarılı bir denetim kaldırır (SaveAudit).
func (tx *Tx) MarkReaudit(ctx context.Context, ids []uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT id FROM invoices WHERE tenant_id=$1 AND id = ANY($2) ORDER BY id FOR UPDATE`, tx.tenantID, ids)
	if err != nil {
		return wrap("lock reaudit invoices", err)
	}
	_, err = tx.Exec(ctx, `UPDATE invoices SET reaudit_since=COALESCE(reaudit_since, now()) WHERE tenant_id=$1 AND id = ANY($2)`,
		tx.tenantID, ids)
	return wrap("mark reaudit", err)
}

// StuckInvoices, 5 dakikadan uzun süredir PENDING kalan ya da yeniden denetim bekleyen faturaları en eskiden başlayarak
// döner. Daha yenilerin denetimi hâlâ kendi isteğinde sürüyor olabilir.
func (tx *Tx) StuckInvoices(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM invoices WHERE tenant_id=$1
		AND (audit_status='PENDING' AND created_at < now() - interval '5 minutes' OR reaudit_since < now() - interval '5 minutes')
		ORDER BY created_at, id`, tx.tenantID)
	if err != nil {
		return nil, wrap("stuck invoices", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return ids, wrap("stuck invoices", err)
}

// VendorInvoicesBetween, tedarikçinin düzenleme tarihi [from, until] aralığında olan faturalarını döner.
func (tx *Tx) VendorInvoicesBetween(ctx context.Context, vendorID uuid.UUID, from, until time.Time) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM invoices WHERE tenant_id=$1 AND vendor_id=$2 AND issue_date BETWEEN $3 AND $4`,
		tx.tenantID, vendorID, from, until)
	if err != nil {
		return nil, wrap("vendor invoices", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return ids, wrap("vendor invoices", err)
}
