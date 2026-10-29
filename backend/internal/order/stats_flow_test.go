package order_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/stats"
	"gp/test/apptest"
)

type dayRow struct {
	Submitted, Paid, Delivered, Rejected, Expired, Cancelled int
	GMV, ShopItems, Income, Discount                         int64
	ReviewN, PickN                                           int
}

// statsRow reads the shop's row of today after the events ran.
func (f fixture) statsRow(shop uuid.UUID) dayRow {
	f.a.T.Helper()
	if err := f.a.DrainEvents(); err != nil {
		f.a.T.Fatal(err)
	}
	var r dayRow
	err := f.a.Env.Owner.QueryRow(context.Background(), `
		SELECT orders_submitted, orders_paid, orders_delivered, orders_rejected, orders_expired, orders_cancelled,
		       gmv_cents, shop_items_cents, platform_income_cents, discount_cents, review_count, pick_count
		FROM shop_daily_stats WHERE shop_id = $1 AND day = $2::date`, shop, today()).
		Scan(&r.Submitted, &r.Paid, &r.Delivered, &r.Rejected, &r.Expired, &r.Cancelled, &r.GMV, &r.ShopItems, &r.Income, &r.Discount, &r.ReviewN, &r.PickN)
	if err != nil {
		f.a.T.Fatalf("no stats row: %v", err)
	}
	return r
}

func (f fixture) dashboard(c apptest.Client, query string) stats.ShopDashboard {
	f.a.T.Helper()
	return apptest.Decode[stats.ShopDashboard](f.a.T, c.OK(http.MethodGet, "/console/shops/"+f.shop.String()+"/dashboard"+query, ""))
}

// FR-DASH-05: the day's row follows the order through its events, and the
// dashboard shows the same numbers as the order and the ledger.
func TestStats_DeliveredOrderMatchesTheDashboard(t *testing.T) {
	f := newFixture(t)
	s := f.delivered()
	var total, items, margin, service, discount int64
	if err := f.a.Env.Owner.QueryRow(t.Context(), `SELECT total_cents, items_cents, platform_margin_cents, service_fee_cents, discount_cents FROM orders WHERE id = $1`, s.id).
		Scan(&total, &items, &margin, &service, &discount); err != nil {
		t.Fatal(err)
	}
	row := f.statsRow(f.shop)
	want := dayRow{Submitted: 1, Paid: 1, Delivered: 1, GMV: total, ShopItems: items - margin, Income: margin + service, Discount: discount, ReviewN: 1, PickN: 1}
	if row != want {
		t.Fatalf("row = %+v, want %+v", row, want)
	}
	if again := f.statsRow(f.shop); again != want {
		t.Errorf("running the events twice changed the row: %+v", again)
	}

	owner := f.staff("owner1", "SHOP_OWNER")
	d := f.dashboard(owner, "")
	if d.Full == nil {
		t.Fatal("the owner gets no money numbers")
	}
	full := d.Full
	if int64(full.SalesCents) != total || full.Orders != 1 || full.Delivered != 1 || int64(full.AvgBillCents) != total || full.CancelRateBps != 0 {
		t.Errorf("full = %+v", full)
	}
	if int64(full.MarginCents) != margin || int64(full.PayoutDueCents) != f.shopBalance() || int64(full.ShopItemsCents) != items-margin {
		t.Errorf("margin %d / payout due %d / shop items %d", full.MarginCents, full.PayoutDueCents, full.ShopItemsCents)
	}
	if len(full.Days) != 1 || full.Days[0].Day != today() || int64(full.Days[0].SalesCents) != total {
		t.Errorf("days = %+v", full.Days)
	}
	if len(full.TopProducts) == 0 || full.TopProducts[0].Quantity == "" || full.TopProducts[0].SalesCents <= 0 {
		t.Errorf("top products = %+v", full.TopProducts)
	}
	for status, n := range d.Queue {
		if n != 0 {
			t.Errorf("%s has %d orders, the order is delivered", status, n)
		}
	}

	// The staff see the queue and the speed, never the money.
	if st := f.dashboard(f.staff("ravi2", "SHOP_STAFF"), ""); st.Full != nil || len(st.Queue) == 0 {
		t.Errorf("staff dashboard = %+v", st)
	}
	// A driver is not shop staff.
	f.staff("kamal2", "SHOP_DRIVER").Expect(http.MethodGet, "/console/shops/"+f.shop.String()+"/dashboard", "", http.StatusForbidden, "FORBIDDEN")
}

// The queue counts the open orders of the shop now.
func TestStats_QueueCountsOpenOrders(t *testing.T) {
	f := newFixture(t)
	f.paidFlow()
	f.secondOrder() // approved, waiting for payment
	q := f.dashboard(f.staff("ravi3", "SHOP_STAFF"), "").Queue
	if q["AWAITING_PAYMENT"] != 1 || q["PAID"] != 1 || q["PICKING"] != 0 {
		t.Errorf("queue = %v", q)
	}
}

// A cancelled order counts as cancelled and, with nothing else submitted that
// day, makes the cancel rate 100 %.
func TestStats_CancelledOrder(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	f.me.OK(http.MethodPost, "/orders/"+x.o.ID.String()+"/cancel", `{"version":1}`)
	row := f.statsRow(f.shop)
	if row.Submitted != 1 || row.Cancelled != 1 || row.Paid != 0 {
		t.Fatalf("row = %+v", row)
	}
	d := f.dashboard(f.staff("owner2", "SHOP_OWNER"), "")
	if d.Full.CancelRateBps != 10000 || d.Full.Orders != 0 || d.Full.AvgBillCents != 0 {
		t.Errorf("full = %+v", d.Full)
	}
}
