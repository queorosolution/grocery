// Package stats keeps the daily numbers of docs/05 section 7 (shop_daily_stats)
// and reads them for the dashboards (docs/06 section 8). Consumers add to one
// row per shop and Colombo day; a dashboard never counts orders.
package stats

import "gp/internal/shared/money"

// Sums are the counters of shop_daily_stats added over some days. The same shape
// is one event's change (a delta) and a dashboard's total.
type Sums struct {
	Submitted, Paid, Delivered, Rejected, Expired, Cancelled int
	GMVCents, ShopItemsCents, PlatformIncomeCents            money.Cents
	DiscountCents                                            money.Cents
	ReviewSeconds, PickSeconds                               int64
	ReviewCount, PickCount                                   int
}

// fields lists the columns in the order of sumCols, so one scan fills a Sums.
func (s *Sums) fields() []any {
	return []any{&s.Submitted, &s.Paid, &s.Delivered, &s.Rejected, &s.Expired, &s.Cancelled,
		&s.GMVCents, &s.ShopItemsCents, &s.PlatformIncomeCents, &s.DiscountCents,
		&s.ReviewSeconds, &s.ReviewCount, &s.PickSeconds, &s.PickCount}
}

// sumCols are the columns of Sums as sums, for a query that adds rows up.
const sumCols = `COALESCE(sum(orders_submitted), 0)::int, COALESCE(sum(orders_paid), 0)::int, COALESCE(sum(orders_delivered), 0)::int,
	COALESCE(sum(orders_rejected), 0)::int, COALESCE(sum(orders_expired), 0)::int, COALESCE(sum(orders_cancelled), 0)::int,
	COALESCE(sum(gmv_cents), 0)::bigint, COALESCE(sum(shop_items_cents), 0)::bigint, COALESCE(sum(platform_income_cents), 0)::bigint,
	COALESCE(sum(discount_cents), 0)::bigint, COALESCE(sum(review_seconds_sum), 0)::bigint, COALESCE(sum(review_count), 0)::int,
	COALESCE(sum(pick_seconds_sum), 0)::bigint, COALESCE(sum(pick_count), 0)::int`

// AvgBillCents is the sales per paid order, rounded down; 0 with no order.
func (s Sums) AvgBillCents() money.Cents {
	if s.Paid == 0 {
		return 0
	}
	return s.GMVCents / money.Cents(s.Paid)
}

// CancelRateBps is the share of submitted orders that ended as cancelled,
// rejected or expired, in basis points (100 = 1%) and at most 10000. The orders
// of a day can end on another day, so the share is only exact over long ranges.
func (s Sums) CancelRateBps() int {
	if s.Submitted == 0 {
		return 0
	}
	return min(10000, (s.Cancelled+s.Rejected+s.Expired)*10000/s.Submitted)
}

// AvgReviewSeconds is how long the shop took to answer a bill, on average.
func (s Sums) AvgReviewSeconds() int64 { return average(s.ReviewSeconds, s.ReviewCount) }

// AvgPickSeconds is how long picking took, from PICKING to PACKED, on average.
func (s Sums) AvgPickSeconds() int64 { return average(s.PickSeconds, s.PickCount) }

func average(sum int64, n int) int64 {
	if n == 0 {
		return 0
	}
	return sum / int64(n)
}

// Day is one point of the sales chart.
type Day struct {
	Day        string      `json:"day"`
	SalesCents money.Cents `json:"salesCents"`
	Orders     int         `json:"orders"`
}
