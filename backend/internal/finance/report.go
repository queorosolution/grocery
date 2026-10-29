package finance

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/ids"
	"gp/internal/shared/money"
)

// ReportTotals are what the ledger shows for the dashboards between two times.
// Every number is positive: refunds are already taken off.
type ReportTotals struct {
	MarginCents           money.Cents `json:"marginCents"`
	ServiceFeeCents       money.Cents `json:"serviceFeeCents"`
	PlatformDiscountCents money.Cents `json:"platformDiscountCents"` // coupons the platform paid for
}

// Report adds the ledger rows written in [from, to). A Nil shop means every shop.
// The shop version uses the (shop, time) index; the platform version is for the
// admin's dashboard only.
func Report(ctx context.Context, tx *db.Tx, shopID uuid.UUID, from, to time.Time) (ReportTotals, error) {
	var r ReportTotals
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_cents) FILTER (WHERE type = 'PLATFORM_MARGIN'), 0)::bigint,
		       COALESCE(sum(amount_cents) FILTER (WHERE type = 'SERVICE_FEE'), 0)::bigint,
		       COALESCE(-sum(amount_cents) FILTER (WHERE type = 'DISCOUNT' AND party = 'PLATFORM'), 0)::bigint
		FROM ledger_entries
		WHERE ($1::uuid IS NULL OR shop_id = $1) AND created_at >= $2 AND created_at < $3`,
		ids.Ref(shopID), from, to).Scan(&r.MarginCents, &r.ServiceFeeCents, &r.PlatformDiscountCents)
	return r, err
}
