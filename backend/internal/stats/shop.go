package stats

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/finance"
	"gp/internal/order"
	"gp/internal/shared/authz"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// topProducts is the length of the best-seller list (FR-DASH-01).
const topProducts = 10

// ShopOps is what everyone in the console may see: the queue now and how fast
// the shop works (FR-DASH-02).
type ShopOps struct {
	Queue            map[enum.OrderStatus]int `json:"queue"`
	AvgReviewSeconds int64                    `json:"avgReviewSeconds"`
	AvgPickSeconds   int64                    `json:"avgPickSeconds"`
}

// ShopFull is the owner's part (FR-DASH-01): sales and money.
type ShopFull struct {
	SalesCents     money.Cents        `json:"salesCents"`
	Orders         int                `json:"orders"`
	Delivered      int                `json:"delivered"`
	AvgBillCents   money.Cents        `json:"avgBillCents"`
	CancelRateBps  int                `json:"cancelRateBps"`
	ShopItemsCents money.Cents        `json:"shopItemsCents"`
	DiscountCents  money.Cents        `json:"discountCents"`
	MarginCents    money.Cents        `json:"marginCents"`
	PayoutDueCents money.Cents        `json:"payoutDueCents"`
	Days           []Day              `json:"days"`
	TopProducts    []order.TopProduct `json:"topProducts"`
}

// ShopDashboard is the console's dashboard. Full is nil for a role without
// dashboard.shop_full, so the staff never receive money they may not see.
type ShopDashboard struct {
	From string `json:"from"`
	To   string `json:"to"`
	ShopOps
	Full *ShopFull `json:"full,omitempty"`
}

// ShopDashboard reads the shop's numbers for the range. The shop is scope.ShopID,
// set by the guard. The money part needs dashboard.shop_full, which the role of
// the scope decides.
func (s *Service) ShopDashboard(ctx context.Context, scope db.Scope, fromText, toText string) (ShopDashboard, error) {
	r, err := parseRange(fromText, toText)
	if err != nil {
		return ShopDashboard{}, err
	}
	out := ShopDashboard{From: r.fromText(), To: r.toText()}
	full := authz.RoleHas(scope.Role, enum.PermissionDashboardShopFull)
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		sums, err := totalOf(ctx, tx, &scope.ShopID, r)
		if err != nil {
			return err
		}
		out.AvgReviewSeconds, out.AvgPickSeconds = sums.AvgReviewSeconds(), sums.AvgPickSeconds()
		if out.Queue, err = s.orders.QueueCounts(ctx, tx, scope.ShopID); err != nil || !full {
			return err
		}
		out.Full, err = s.shopFull(ctx, tx, scope.ShopID, r, sums)
		return err
	})
	return out, err
}

func (s *Service) shopFull(ctx context.Context, tx *db.Tx, shopID uuid.UUID, r Range, sums Sums) (*ShopFull, error) {
	f := &ShopFull{
		SalesCents: sums.GMVCents, Orders: sums.Paid, Delivered: sums.Delivered, AvgBillCents: sums.AvgBillCents(),
		CancelRateBps: sums.CancelRateBps(), ShopItemsCents: sums.ShopItemsCents, DiscountCents: sums.DiscountCents,
	}
	var err error
	if f.Days, err = daysOf(ctx, tx, &shopID, r); err != nil {
		return nil, err
	}
	if f.TopProducts, err = s.orders.TopProducts(ctx, tx, shopID, r.From, r.End(), topProducts); err != nil {
		return nil, err
	}
	rep, err := finance.Report(ctx, tx, shopID, r.From, r.End())
	if err != nil {
		return nil, err
	}
	f.MarginCents = rep.MarginCents
	// What the platform owes the shop now, not only in the range.
	f.PayoutDueCents, err = finance.Balance(ctx, tx, shopID, time.Time{})
	return f, err
}
