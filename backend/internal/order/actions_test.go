package order

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

func TestActions(t *testing.T) {
	d := func(s enum.OrderStatus, f func(*orderData)) orderData {
		o := sampleOrder(s)
		o.Coupon = nil
		if f != nil {
			f(&o)
		}
		return o
	}
	paid := func(o *orderData) { at := o.Steps[0].At; o.PaidAt = &at }
	noDriver := func(o *orderData) { o.DriverID = nil }
	withCoupon := func(o *orderData) { o.Coupon = &CouponRef{Code: "WELCOME10"} }
	customer := db.Scope{Role: enum.RoleCustomer}
	owner := db.Scope{UserID: uuid.New(), Role: enum.RoleShopOwner}
	admin := db.Scope{UserID: uuid.New(), Role: enum.RoleSuperAdmin, Privileged: true}

	cases := []struct {
		name  string
		order orderData
		v     Viewer
		sc    db.Scope
		want  []Action
	}{
		{"customer, waiting for the shop", d(enum.OrderStatusSubmitted, nil), ViewCustomer, customer, []Action{ActionCancel}},
		{"customer, revised", d(enum.OrderStatusRevised, nil), ViewCustomer, customer, []Action{ActionAcceptRevision, ActionCancel}},
		{"customer, to pay", d(enum.OrderStatusAwaitingPayment, nil), ViewCustomer, customer, []Action{ActionPay, ActionApplyCoupon, ActionCancel}},
		{"customer, to pay with a coupon", d(enum.OrderStatusAwaitingPayment, withCoupon), ViewCustomer, customer, []Action{ActionPay, ActionRemoveCoupon, ActionCancel}},
		{"customer, paid", d(enum.OrderStatusPicking, paid), ViewCustomer, customer, []Action{ActionReportProblem}},
		{"customer, expired", d(enum.OrderStatusExpired, nil), ViewCustomer, customer, []Action{}},
		{"owner, new bill", d(enum.OrderStatusSubmitted, nil), ViewConsole, owner, []Action{ActionApprove, ActionRevise, ActionReject}},
		{"owner, paid", d(enum.OrderStatusPaid, nil), ViewConsole, owner, []Action{ActionStartPicking, ActionAssignDriver, ActionRequestCancel}},
		{"owner, picking", d(enum.OrderStatusPicking, nil), ViewConsole, owner, []Action{ActionMarkPacked, ActionAssignDriver, ActionRequestCancel, ActionRequestPartialRefund}},
		{"owner, packed without a driver", d(enum.OrderStatusPacked, noDriver), ViewConsole, owner, []Action{ActionAssignDriver}},
		{"owner, packed with a driver", d(enum.OrderStatusPacked, nil), ViewConsole, owner, []Action{ActionAssignDriver, ActionSendOut}},
		{"owner, failed delivery", d(enum.OrderStatusDeliveryFailed, nil), ViewConsole, owner, []Action{ActionAssignDriver, ActionRetryDelivery}},
		{"admin, new bill", d(enum.OrderStatusSubmitted, nil), ViewConsole, admin, []Action{ActionApprove, ActionRevise, ActionReject}},
		{"admin, paid (staff start picking)", d(enum.OrderStatusPaid, nil), ViewConsole, admin, []Action{ActionAssignDriver}},
	}
	for _, c := range cases {
		if got := c.order.actions(c.v, c.sc); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	out := d(enum.OrderStatusOutForDelivery, nil)
	mine := db.Scope{UserID: *out.DriverID, Role: enum.RoleShopDriver}
	if got := out.actions(ViewDriver, mine); !slices.Equal(got, []Action{ActionComplete, ActionFail}) {
		t.Errorf("assigned driver: %v", got)
	}
	if got := out.actions(ViewDriver, db.Scope{UserID: uuid.New(), Role: enum.RoleShopDriver}); len(got) != 0 {
		t.Errorf("another driver: %v, want none", got)
	}
	if got := d(enum.OrderStatusPacked, nil).actions(ViewDriver, mine); len(got) != 0 {
		t.Errorf("driver before the order is out: %v", got)
	}
}
