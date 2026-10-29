package stats

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/connector"
	"gp/internal/finance"
	"gp/internal/notify"
	"gp/internal/payment"
	"gp/internal/shared/db"
	"gp/internal/shared/money"
)

// maxShopRows bounds the per-shop table of the admin's dashboard.
const maxShopRows = 500

// ShopRow is one shop's line in the admin's table.
type ShopRow struct {
	ShopID              uuid.UUID   `json:"shopId"`
	Name                string      `json:"name"`
	SalesCents          money.Cents `json:"salesCents"`
	Orders              int         `json:"orders"`
	Delivered           int         `json:"delivered"`
	PlatformIncomeCents money.Cents `json:"platformIncomeCents"`
}

// AdminDashboard is FR-DASH-04: all shops, the platform's money, SMS, failed
// payments and the health of the syncs.
type AdminDashboard struct {
	From                string           `json:"from"`
	To                  string           `json:"to"`
	GMVCents            money.Cents      `json:"gmvCents"`
	Orders              int              `json:"orders"`
	Delivered           int              `json:"delivered"`
	AvgBillCents        money.Cents      `json:"avgBillCents"`
	CancelRateBps       int              `json:"cancelRateBps"`
	PlatformIncomeCents money.Cents      `json:"platformIncomeCents"`
	MarginCents         money.Cents      `json:"marginCents"`
	ServiceFeeCents     money.Cents      `json:"serviceFeeCents"`
	CouponCostCents     money.Cents      `json:"couponCostCents"`
	SMSSent             int              `json:"smsSent"`
	FailedPayments      int              `json:"failedPayments"`
	Sync                connector.Health `json:"sync"`
	Days                []Day            `json:"days"`
	Shops               []ShopRow        `json:"shops"`
}

// AdminDashboard reads the whole platform for the range. The caller is the admin
// guard's scope. The platform income and the shop table come from the stats
// table; the split of the income, the coupon cost, the SMS, the failed payments
// and the sync health are small counts from the modules that own them.
func (s *Service) AdminDashboard(ctx context.Context, scope db.Scope, fromText, toText string) (AdminDashboard, error) {
	r, err := parseRange(fromText, toText)
	if err != nil {
		return AdminDashboard{}, err
	}
	out := AdminDashboard{From: r.fromText(), To: r.toText()}
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		sums, err := totalOf(ctx, tx, nil, r)
		if err != nil {
			return err
		}
		out.GMVCents, out.Orders, out.Delivered, out.AvgBillCents = sums.GMVCents, sums.Paid, sums.Delivered, sums.AvgBillCents()
		out.CancelRateBps, out.PlatformIncomeCents = sums.CancelRateBps(), sums.PlatformIncomeCents
		if out.Days, err = daysOf(ctx, tx, nil, r); err != nil {
			return err
		}
		if out.Shops, err = shopRows(ctx, tx, r); err != nil {
			return err
		}
		return s.platformNumbers(ctx, tx, r, &out)
	})
	return out, err
}

func (s *Service) platformNumbers(ctx context.Context, tx *db.Tx, r Range, out *AdminDashboard) error {
	rep, err := finance.Report(ctx, tx, uuid.Nil, r.From, r.End())
	if err != nil {
		return err
	}
	out.MarginCents, out.ServiceFeeCents, out.CouponCostCents = rep.MarginCents, rep.ServiceFeeCents, rep.PlatformDiscountCents
	if out.SMSSent, err = notify.SentBetween(ctx, tx, r.From, r.End()); err != nil {
		return err
	}
	if out.FailedPayments, err = payment.FailedBetween(ctx, tx, r.From, r.End()); err != nil {
		return err
	}
	out.Sync, err = connector.HealthNow(ctx, tx)
	return err
}

// shopRows is the table of shops that had any sales or orders, best seller first.
func shopRows(ctx context.Context, tx *db.Tx, r Range) ([]ShopRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT st.shop_id, sh.name, sum(st.gmv_cents)::bigint, sum(st.orders_paid)::int, sum(st.orders_delivered)::int, sum(st.platform_income_cents)::bigint
		FROM shop_daily_stats st JOIN shops sh ON sh.id = st.shop_id
		WHERE st.day BETWEEN $1::date AND $2::date
		GROUP BY st.shop_id, sh.name HAVING sum(st.gmv_cents) > 0 OR sum(st.orders_paid) > 0
		ORDER BY 3 DESC, 2 LIMIT $3`, r.fromText(), r.toText(), maxShopRows)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ShopRow{}
	for rows.Next() {
		var x ShopRow
		if err := rows.Scan(&x.ShopID, &x.Name, &x.SalesCents, &x.Orders, &x.Delivered, &x.PlatformIncomeCents); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
