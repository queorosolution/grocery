package order_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/test/fakes"
)

// submitted makes a verified customer's SUBMITTED order with one booked place.
func (f fixture) submitted() (order.Detail, uuid.UUID) {
	f.a.T.Helper()
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	slot := f.firstSlot(f.shop)
	return f.placed(f.me, key1, f.body(`"slotId":"`+slot.String()+`"`, rice)), slot
}

func (f fixture) userID(sub string) uuid.UUID {
	return f.a.SignedIn(fakes.Claims{Sub: sub, Email: sub + "@example.com", Name: sub}).Me.ID
}

func codeOf(err error) apperr.Code {
	var app *apperr.Error
	if errors.As(err, &app) {
		return app.Code
	}
	return ""
}

// TestTransition_Table runs every pair of statuses for every actor: Transition
// agrees with the generated table, and a move whose side effects are not built
// yet refuses to run instead of changing the order without them.
func TestTransition_Table(t *testing.T) {
	f := newFixture(t)
	o, slot := f.submitted()
	driver, staff := f.userID("driver"), f.userID("staff")
	f.a.Grant(driver, &f.shop, string(enum.RoleShopDriver)) // T15 and T18 need a driver who may deliver
	actors := map[string]db.Scope{
		"CUSTOMER":        {UserID: f.me.Session.Me.ID, Role: enum.RoleCustomer},
		"SHOP_OWNER":      {UserID: staff, ShopID: f.shop, Role: enum.RoleShopOwner},
		"SHOP_STAFF":      {UserID: staff, ShopID: f.shop, Role: enum.RoleShopStaff},
		"SUPER_ADMIN":     {UserID: staff, Privileged: true, Role: enum.RoleSuperAdmin},
		"SYSTEM":          {Privileged: true},
		"SHOP_DRIVER":     {UserID: driver, Role: enum.RoleShopDriver},
		"PLATFORM_DRIVER": {UserID: driver, Role: enum.RolePlatformDriver},
	}
	built := map[string]bool{"T02": true, "T04": true, "T05": true, "T06": true, "T07": true, "T08": true, "T09": true, "T10": true, "T11": true, "T12": true, "T13": true, "T14": true, "T15": true, "T17": true, "T18": true, "T19": true}
	ran := 0
	for _, from := range enum.OrderStatusValues() {
		for _, to := range enum.OrderStatusValues() {
			for name, sc := range actors {
				f.a.Exec(`UPDATE orders SET status = $2, version = 1, driver_id = $3 WHERE id = $1`, o.ID, from, driver)
				f.a.Exec(`UPDATE slots SET booked = 1 WHERE id = $1`, slot)
				_, err := f.a.Orders.Transition(t.Context(), sc, o.ID, to, order.TransitionInput{Version: 1, Reason: "because"})
				label := fmt.Sprintf("%s -> %s by %s", from, to, name)
				tr, exists := order.Lookup(from, to)
				switch {
				case !exists || !tr.AllowedFor(name):
					if codeOf(err) != apperr.InvalidTransition {
						t.Errorf("%s: err = %v, want INVALID_TRANSITION", label, err)
					}
				case tr.ID == "T16" && name != "SUPER_ADMIN":
					// Only the delivery code completes a delivery (BIZ-11); see TestDeliveryCode_*.
					if codeOf(err) != apperr.InvalidTransition {
						t.Errorf("%s (T16): err = %v, want INVALID_TRANSITION without the code", label, err)
					}
				case tr.ID == "T16":
					ran++
					if err != nil {
						t.Errorf("%s (T16): %v", label, err)
					}
				case tr.ID == "T03":
					// A revision with no changes is refused; Review tests cover the rest.
					if codeOf(err) != apperr.ValidationError {
						t.Errorf("%s (T03): err = %v, want VALIDATION_ERROR", label, err)
					}
				case built[tr.ID]:
					ran++
					if err != nil {
						t.Errorf("%s (%s): %v", label, tr.ID, err)
					} else if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = $2 AND version = 2`, o.ID, string(to)) != 1 {
						t.Errorf("%s: the order did not move", label)
					}
				default:
					if err == nil || !strings.Contains(err.Error(), tr.ID) {
						t.Errorf("%s (%s): err = %v, want a refusal naming the transition", label, tr.ID, err)
					}
				}
			}
		}
	}
	if ran == 0 {
		t.Fatal("no move ran")
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'order.transition' AND actor_role = 'SUPER_ADMIN'`) == 0 {
		t.Error("an admin's moves are not audited")
	}
}

func TestTransition_StaleVersion(t *testing.T) {
	f := newFixture(t)
	o, slot := f.submitted()
	me := db.Scope{UserID: f.me.Session.Me.ID, Role: enum.RoleCustomer}
	_, err := f.a.Orders.Transition(t.Context(), me, o.ID, enum.OrderStatusCancelled, order.TransitionInput{Version: 7})
	if codeOf(err) != apperr.StaleOrder {
		t.Fatalf("wrong version: %v, want STALE_ORDER", err)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE status = 'SUBMITTED' AND version = 1`) != 1 {
		t.Fatal("a stale request changed the order")
	}
	got, err := f.a.Orders.Transition(t.Context(), me, o.ID, enum.OrderStatusCancelled, order.TransitionInput{Version: 1})
	if err != nil || got.Status != enum.OrderStatusCancelled || got.Version != 2 || len(got.Actions) != 0 {
		t.Fatalf("cancel = %+v, %v", got, err)
	}
	if f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot) != 0 ||
		f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND closed_at IS NOT NULL`, o.ID) != 1 ||
		f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.cancelled' AND entity_id = $1`, o.ID.String()) != 1 ||
		f.a.Count(`SELECT count(*) FROM order_events WHERE order_id = $1 AND from_status = 'SUBMITTED' AND to_status = 'CANCELLED'`, o.ID) != 1 {
		t.Error("the cancel did not release the place, close the order, or write its history and event")
	}
	_, err = f.a.Orders.Transition(t.Context(), me, o.ID, enum.OrderStatusCancelled, order.TransitionInput{Version: 1})
	if codeOf(err) != apperr.StaleOrder {
		t.Errorf("same request again: %v, want STALE_ORDER", err)
	}
	_, err = f.a.Orders.Transition(t.Context(), me, o.ID, enum.OrderStatusCancelled, order.TransitionInput{Version: 2})
	if codeOf(err) != apperr.InvalidTransition {
		t.Errorf("from a final status: %v, want INVALID_TRANSITION", err)
	}
}

// Two requests for the same version: one wins, the other is stale.
func TestTransition_RaceHasOneWinner(t *testing.T) {
	f := newFixture(t)
	o, slot := f.submitted()
	me := db.Scope{UserID: f.me.Session.Me.ID, Role: enum.RoleCustomer}
	errs := make([]error, 6)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.a.Orders.Transition(t.Context(), me, o.ID, enum.OrderStatusCancelled, order.TransitionInput{Version: 1})
		}()
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		if err == nil {
			won++
		} else if codeOf(err) != apperr.StaleOrder {
			t.Errorf("loser got %v, want STALE_ORDER", err)
		}
	}
	if won != 1 || f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot) != 0 {
		t.Errorf("winners = %d, booked = %d; want 1 winner and the place released once", won, f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot))
	}
}

func TestTransition_ScopeAndReason(t *testing.T) {
	f := newFixture(t)
	o, _ := f.submitted()
	stranger := db.Scope{UserID: f.userID("kamal"), Role: enum.RoleCustomer}
	otherShop, _ := f.readyShop("other")
	theirStaff := db.Scope{UserID: f.userID("theirs"), ShopID: otherShop, Role: enum.RoleShopOwner}
	ownStaff := db.Scope{UserID: f.userID("mine"), ShopID: f.shop, Role: enum.RoleShopOwner}
	for name, sc := range map[string]db.Scope{"another customer": stranger, "another shop's owner": theirStaff} {
		if _, err := f.a.Orders.Transition(t.Context(), sc, o.ID, enum.OrderStatusRejected, order.TransitionInput{Version: 1, Reason: "no"}); codeOf(err) != apperr.NotFound {
			t.Errorf("%s: %v, want NOT_FOUND", name, err)
		}
	}
	if _, err := f.a.Orders.Transition(t.Context(), ownStaff, o.ID, enum.OrderStatusRejected, order.TransitionInput{Version: 1}); codeOf(err) != apperr.ValidationError {
		t.Errorf("reject with no reason: %v, want VALIDATION_ERROR", err)
	}
	long := strings.Repeat("x", 301)
	if _, err := f.a.Orders.Transition(t.Context(), ownStaff, o.ID, enum.OrderStatusRejected, order.TransitionInput{Version: 1, Reason: long}); codeOf(err) != apperr.ValidationError {
		t.Errorf("reject with a long reason: %v, want VALIDATION_ERROR", err)
	}
	got, err := f.a.Orders.Transition(t.Context(), ownStaff, o.ID, enum.OrderStatusRejected, order.TransitionInput{Version: 1, Reason: "out of stock"})
	if err != nil || got.Status != enum.OrderStatusRejected || got.Customer == nil || strings.Contains(got.Customer.PhoneMasked, "7712") {
		t.Fatalf("reject = %+v, %v", got, err)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status_reason = 'out of stock' AND decided_at IS NOT NULL`, o.ID) != 1 {
		t.Error("the reason or the decision time is missing")
	}
}
