package order

import (
	"testing"

	"gp/internal/shared/enum"
)

func TestAllowed(t *testing.T) {
	cases := []struct {
		name     string
		from, to enum.OrderStatus
		actor    string
		want     bool
	}{
		{"customer submits", "", enum.OrderStatusSubmitted, "CUSTOMER", true},
		{"shop cannot submit", "", enum.OrderStatusSubmitted, "SHOP_OWNER", false},
		{"staff approves", enum.OrderStatusSubmitted, enum.OrderStatusAwaitingPayment, "SHOP_STAFF", true},
		{"customer cannot approve", enum.OrderStatusSubmitted, enum.OrderStatusAwaitingPayment, "CUSTOMER", false},
		{"timer expires", enum.OrderStatusSubmitted, enum.OrderStatusExpired, "SYSTEM", true},
		{"customer cannot cancel after payment", enum.OrderStatusPaid, enum.OrderStatusCancelled, "CUSTOMER", false},
		{"only admin refunds a paid order", enum.OrderStatusPaid, enum.OrderStatusCancelledRefund, "SUPER_ADMIN", true},
		{"shop cannot refund a paid order", enum.OrderStatusPaid, enum.OrderStatusCancelledRefund, "SHOP_OWNER", false},
		{"no way out of a final status", enum.OrderStatusDelivered, enum.OrderStatusPaid, "SUPER_ADMIN", false},
	}
	for _, tc := range cases {
		if got := Allowed(tc.from, tc.to, tc.actor); got != tc.want {
			t.Errorf("%s: Allowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestReasonRequired(t *testing.T) {
	tr, ok := Lookup(enum.OrderStatusSubmitted, enum.OrderStatusRejected)
	if !ok || !tr.ReasonRequired {
		t.Fatalf("rejecting a bill must require a reason, got %+v", tr)
	}
}

func TestIsFinalHasNoOutgoingMoves(t *testing.T) {
	for _, s := range enum.OrderStatusValues() {
		for _, tr := range Transitions() {
			if IsFinal(s) && tr.From == s {
				t.Errorf("%s is final but %s leaves it", s, tr.ID)
			}
		}
	}
	if !IsFinal(enum.OrderStatusDelivered) || IsFinal(enum.OrderStatusPaid) {
		t.Error("IsFinal gives the wrong answer for DELIVERED or PAID")
	}
}
