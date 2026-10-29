package order_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

func cancelPath(id uuid.UUID) string { return "/orders/" + id.String() + "/cancel" }

// The customer may withdraw while the shop or they are the ones to move (T06, T09, T12).
func TestCancel_BeforePayment(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(f fixture, shop apptest.Client, x trio) // moves the order; the version after is 2
		version int
	}{
		{"waiting for the shop", func(fixture, apptest.Client, trio) {}, 1},
		{"revised", func(f fixture, shop apptest.Client, x trio) {
			f.review(shop, x.o.ID, version(1, changes(qtyOf(x.rice0.ID, 2))))
		}, 2},
		{"waiting for payment", func(f fixture, shop apptest.Client, x trio) {
			f.review(shop, x.o.ID, version(1, `"action":"APPROVE"`))
		}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			x := f.trio()
			c.prepare(f, f.staff("asha", "SHOP_STAFF"), x)
			if f.booked(x.slot) != 1 {
				t.Fatal("the order holds no place")
			}
			f.me.Expect(http.MethodPost, cancelPath(x.o.ID), `{"version":0}`, http.StatusBadRequest, "VALIDATION_ERROR")
			f.me.Expect(http.MethodPost, cancelPath(x.o.ID), `{"version":99}`, http.StatusConflict, "STALE_ORDER")
			got := apptest.Decode[order.Detail](t, f.me.OK(http.MethodPost, cancelPath(x.o.ID), version(c.version, "")))
			if got.Status != enum.OrderStatusCancelled || len(got.Actions) != 0 {
				t.Errorf("cancelled = %+v", got)
			}
			if f.booked(x.slot) != 0 || f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.cancelled' AND entity_id = $1`, x.o.ID.String()) != 1 {
				t.Error("the place was not released or order.cancelled was not written")
			}
			f.me.Expect(http.MethodPost, cancelPath(x.o.ID), version(c.version+1, ""), http.StatusConflict, "INVALID_TRANSITION")
		})
	}
}

// BIZ-10: once paid, no route lets the customer cancel.
func TestCancel_AfterPaymentIsRefused(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	for _, status := range []enum.OrderStatus{enum.OrderStatusPaid, enum.OrderStatusPicking, enum.OrderStatusPacked,
		enum.OrderStatusOutForDelivery, enum.OrderStatusDeliveryFailed, enum.OrderStatusDelivered} {
		f.a.Exec(`UPDATE orders SET status = $2, version = 5 WHERE id = $1`, x.o.ID, status)
		f.me.Expect(http.MethodPost, cancelPath(x.o.ID), `{"version":5}`, http.StatusConflict, "INVALID_TRANSITION")
		f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":5,"revisionNo":0}`, http.StatusBadRequest, "VALIDATION_ERROR")
		f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":5,"revisionNo":1}`, http.StatusConflict, "STALE_ORDER")
	}
	f.a.Exec(`UPDATE orders SET revision_no = 1 WHERE id = $1`, x.o.ID)
	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":5,"revisionNo":1}`, http.StatusConflict, "INVALID_TRANSITION")
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = 'DELIVERED' AND version = 5`, x.o.ID) != 1 || f.booked(x.slot) != 1 {
		t.Error("a refused cancel changed the order or its slot")
	}
}

// Only the owner of an order cancels it or accepts its revision.
func TestCancel_OnlyTheOwner(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	stranger := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "kamal", Email: "kamal@example.com", Name: "Kamal"}))
	stranger.Expect(http.MethodPost, cancelPath(x.o.ID), `{"version":1}`, http.StatusNotFound, "NOT_FOUND")
	stranger.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":1,"revisionNo":1}`, http.StatusNotFound, "NOT_FOUND")
	stranger.Expect(http.MethodPost, cancelPath(uuid.New()), `{"version":1}`, http.StatusNotFound, "NOT_FOUND")
	f.me.Expect(http.MethodPost, cancelPath(x.o.ID), `{"version":1,"status":"PAID"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	if w := f.a.Call(http.MethodPost, "/v1"+cancelPath(x.o.ID), `{"version":1}`); w.Code != http.StatusUnauthorized {
		t.Errorf("no login = %d, want 401", w.Code)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = 'SUBMITTED'`, x.o.ID) != 1 {
		t.Error("somebody else's cancel changed the order")
	}
}
