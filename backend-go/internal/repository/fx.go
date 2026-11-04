package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bengin-noyan/matchwatch/backend-go/internal/domain"
)

// FXDay reports whether the TCMB bulletin of day was already fetched, whether it existed, and its quotes.
func (tx *Tx) FXDay(ctx context.Context, day time.Time) (known, published bool, quotes []domain.FXQuote, err error) {
	err = tx.QueryRow(ctx, `SELECT published FROM fx_days WHERE rate_date=$1`, day).Scan(&published)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil, nil
	}
	if err != nil || !published {
		return err == nil, false, nil, wrap("fx day", err)
	}
	rows, err := tx.Query(ctx, `SELECT currency, unit, forex_buying::text FROM fx_rates WHERE rate_date=$1 ORDER BY currency`, day)
	if err != nil {
		return false, false, nil, wrap("fx rates", err)
	}
	defer rows.Close()
	for rows.Next() {
		var q domain.FXQuote
		var fb string
		if err := rows.Scan(&q.Currency, &q.Unit, &fb); err != nil {
			return false, false, nil, wrap("scan fx rate", err)
		}
		q.ForexBuying = dec(fb)
		quotes = append(quotes, q)
	}
	return true, true, quotes, rows.Err()
}

// SaveFXDay stores a fetched bulletin (or its absence). A concurrent save of the same day is a no-op.
func (tx *Tx) SaveFXDay(ctx context.Context, day time.Time, published bool, quotes []domain.FXQuote) error {
	tag, err := tx.Exec(ctx, `INSERT INTO fx_days(rate_date, published) VALUES ($1,$2) ON CONFLICT DO NOTHING`, day, published)
	if err != nil || tag.RowsAffected() == 0 {
		return wrap("save fx day", err)
	}
	for _, q := range quotes {
		if _, err := tx.Exec(ctx, `INSERT INTO fx_rates(rate_date, currency, unit, forex_buying) VALUES ($1,$2,$3,$4)`,
			day, q.Currency, q.Unit, num(q.ForexBuying)); err != nil {
			return wrap("save fx rate", err)
		}
	}
	return nil
}
