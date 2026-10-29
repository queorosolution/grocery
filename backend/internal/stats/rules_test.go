package stats

import (
	"testing"
	"time"

	"gp/internal/order"
	"gp/internal/shared/enum"
)

func TestRules_Paid(t *testing.T) {
	f := order.StatFacts{TotalCents: 120000, ItemsCents: 100000, MarginCents: 8000, ServiceFeeCents: 2000, DiscountCents: 5000}
	got := rules[enum.EventTypeOrderPaid](payload{}, f, time.Now())
	want := Sums{Paid: 1, GMVCents: 120000, ShopItemsCents: 92000, PlatformIncomeCents: 10000, DiscountCents: 5000}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRules_ReviewTimeOnlyForTheFirstAnswer(t *testing.T) {
	at := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	f := order.StatFacts{SubmittedAt: at.Add(-90 * time.Second)}
	for _, tc := range []struct {
		name string
		typ  enum.EventType
		from enum.OrderStatus
		want Sums
	}{
		{"approve", enum.EventTypeOrderApproved, enum.OrderStatusSubmitted, Sums{ReviewSeconds: 90, ReviewCount: 1}},
		{"revise", enum.EventTypeOrderRevised, enum.OrderStatusSubmitted, Sums{ReviewSeconds: 90, ReviewCount: 1}},
		{"reject", enum.EventTypeOrderRejected, enum.OrderStatusSubmitted, Sums{Rejected: 1, ReviewSeconds: 90, ReviewCount: 1}},
		{"customer accepts the revision", enum.EventTypeOrderApproved, enum.OrderStatusRevised, Sums{}},
		{"customer rejects the revision", enum.EventTypeOrderRejected, enum.OrderStatusRevised, Sums{Rejected: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rules[tc.typ](payload{From: tc.from}, f, at); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
	// The rule is shared by every event: a first answer must not leak into the next call.
	if got := rules[enum.EventTypeOrderApproved](payload{From: enum.OrderStatusRevised}, f, at); got != (Sums{}) {
		t.Errorf("state leaked between calls: %+v", got)
	}
}

func TestRules_PickTimeWhenPacked(t *testing.T) {
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	start := at.Add(-5 * time.Minute)
	f := order.StatFacts{PickingAt: &start}
	rule := rules[enum.EventTypeOrderStatusChanged]
	if got := rule(payload{To: enum.OrderStatusPacked}, f, at); got != (Sums{PickSeconds: 300, PickCount: 1}) {
		t.Errorf("packed = %+v", got)
	}
	if got := rule(payload{To: enum.OrderStatusPicking}, f, at); got != (Sums{}) {
		t.Errorf("picking changes nothing, got %+v", got)
	}
	if got := rule(payload{To: enum.OrderStatusPacked}, order.StatFacts{}, at); got != (Sums{}) {
		t.Errorf("packed with no picking time = %+v", got)
	}
}

func TestRules_SecondsNeverNegative(t *testing.T) {
	now := time.Now()
	if got := seconds(now.Add(time.Minute), now); got != 0 {
		t.Errorf("got %d", got)
	}
}

// Every event the docs give to the stats has a rule, and no other.
func TestRules_EventsOfDocs04(t *testing.T) {
	for _, typ := range []enum.EventType{
		enum.EventTypeOrderSubmitted, enum.EventTypeOrderRejected, enum.EventTypeOrderExpired, enum.EventTypeOrderCancelled,
		enum.EventTypeOrderPaid, enum.EventTypeOrderStatusChanged, enum.EventTypeOrderDelivered, enum.EventTypeOrderCancelledRefund,
	} {
		if rules[typ] == nil {
			t.Errorf("no rule for %s", typ)
		}
	}
	if rules[enum.EventTypeOrderOutForDelivery] != nil || rules[enum.EventTypeCatalogChanged] != nil {
		t.Error("a rule for an event the stats do not follow")
	}
}

func TestSums_Derived(t *testing.T) {
	s := Sums{Submitted: 8, Paid: 3, GMVCents: 100001, Cancelled: 1, Rejected: 1, Expired: 1, ReviewSeconds: 300, ReviewCount: 4, PickSeconds: 10, PickCount: 3}
	if s.AvgBillCents() != 33333 || s.CancelRateBps() != 3750 || s.AvgReviewSeconds() != 75 || s.AvgPickSeconds() != 3 {
		t.Errorf("got %d %d %d %d", s.AvgBillCents(), s.CancelRateBps(), s.AvgReviewSeconds(), s.AvgPickSeconds())
	}
	if z := (Sums{}); z.AvgBillCents() != 0 || z.CancelRateBps() != 0 || z.AvgReviewSeconds() != 0 || z.AvgPickSeconds() != 0 {
		t.Error("an empty range must give zeros, not a division by zero")
	}
	// Orders of earlier days can be cancelled today: the rate stops at 100 %.
	if over := (Sums{Submitted: 1, Cancelled: 3}); over.CancelRateBps() != 10000 {
		t.Errorf("rate = %d", over.CancelRateBps())
	}
}

func TestParseRange(t *testing.T) {
	r, err := parseRange("2026-10-01", "2026-10-07")
	if err != nil || r.fromText() != "2026-10-01" || r.toText() != "2026-10-07" || !r.End().Equal(r.To.AddDate(0, 0, 1)) {
		t.Fatalf("got %+v, %v", r, err)
	}
	if d, err := parseRange("", ""); err != nil || d.To.Sub(d.From) != 6*24*time.Hour {
		t.Errorf("default range = %+v, %v", d, err)
	}
	for _, tc := range [][2]string{{"x", ""}, {"", "2026-13-40"}, {"2026-10-08", "2026-10-07"}, {"2025-01-01", "2026-10-07"}} {
		if _, err := parseRange(tc[0], tc[1]); err == nil {
			t.Errorf("%v must be refused", tc)
		}
	}
	if _, err := parseRange("2025-10-07", "2026-10-07"); err != nil {
		t.Errorf("366 days must pass: %v", err)
	}
}
