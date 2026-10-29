package order_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/test/apptest"
	"gp/test/fakes"
)

// FR-CPN-03: the use goes back when the order is cancelled, or the payment timer runs out.
func TestCoupon_ReleasedWhenTheOrderEnds(t *testing.T) {
	f := newFixture(t)
	f.makeCoupon(f.a.AsAdmin(), "WELCOME10", "perUserLimit", 5)

	x := f.awaiting()
	decodeApplied(t, f.apply(f.me, x.o.ID, 2, "WELCOME10"))
	f.me.OK(http.MethodPost, cancelPath(x.o.ID), `{"version":3}`)
	if f.used("WELCOME10") != 0 || f.a.Count(`SELECT count(*) FROM coupon_redemptions`) != 0 {
		t.Errorf("cancel: used %d, want the use back", f.used("WELCOME10"))
	}

	second := f.secondOrder()
	decodeApplied(t, f.apply(f.me, second, 2, "WELCOME10"))
	if f.used("WELCOME10") != 1 {
		t.Fatalf("used = %d, want 1", f.used("WELCOME10"))
	}
	if !f.expire(second, events.TimerPayment, 0) {
		t.Fatal("the payment timer did not expire the order")
	}
	if f.used("WELCOME10") != 0 || f.a.Count(`SELECT count(*) FROM coupon_redemptions`) != 0 {
		t.Errorf("expiry: used %d, want the use back", f.used("WELCOME10"))
	}
}

// docs/03 section 11: once the payment confirmed the use, nothing gives it back.
func TestCoupon_ConfirmedUseStays(t *testing.T) {
	f := newFixture(t)
	f.makeCoupon(f.a.AsAdmin(), "WELCOME10")
	x := f.awaiting()
	decodeApplied(t, f.apply(f.me, x.o.ID, 2, "WELCOME10"))

	ctx := context.Background()
	err := f.a.Env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error { return f.a.Coupons.Confirm(ctx, tx, x.o.ID) })
	if err != nil {
		t.Fatal(err)
	}
	var released bool
	err = f.a.Env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		released, err = f.a.Coupons.Release(ctx, tx, x.o.ID)
		return err
	})
	if err != nil || released || f.used("WELCOME10") != 1 || f.a.Count(`SELECT count(*) FROM coupon_redemptions WHERE confirmed_at IS NOT NULL`) != 1 {
		t.Errorf("release of a confirmed use: released %v, used %d, err %v; want it kept", released, f.used("WELCOME10"), err)
	}
}

// awaitingFor makes a new customer with an approved order (one rice) and returns the customer and the order.
func (f fixture) awaitingFor(sub string, rice, slot uuid.UUID, shop apptest.Client) (apptest.Client, uuid.UUID) {
	f.a.T.Helper()
	email := sub + "@example.com"
	c := f.a.As(f.a.SignedIn(fakes.Claims{Sub: sub, Email: email, Name: sub}))
	f.address(c)
	f.verify(email)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	addr := apptest.Decode[struct {
		Data []struct{ ID uuid.UUID } `json:"data"`
	}](f.a.T, c.OK(http.MethodGet, "/me/addresses", "")).Data[0].ID
	o := f.placed(c, "key-for-"+sub+"-0000", f.bodyOf(cartID, addr, `"slotId":"`+slot.String()+`"`, rice))
	if w := f.review(shop, o.ID, version(1, `"action":"APPROVE"`)); w.Code != http.StatusOK {
		f.a.T.Fatalf("approve = %d %s", w.Code, w.Body)
	}
	return c, o.ID
}

// BIZ-03: 20 customers apply a coupon with 5 uses at once; exactly 5 get it.
func TestCoupon_RaceNeverBeatsTheLimit(t *testing.T) {
	f := newFixture(t)
	f.makeCoupon(f.a.AsAdmin(), "FIRST5", "usageLimit", 5)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	slot := f.slot(f.shop, 50)
	shop := f.staff("asha", "SHOP_OWNER")

	const customers = 20
	type buyer struct {
		c  apptest.Client
		id uuid.UUID
	}
	var buyers []buyer
	for i := 0; i < customers; i++ {
		c, id := f.awaitingFor("buyer"+string(rune('a'+i)), rice, slot, shop)
		buyers = append(buyers, buyer{c, id})
	}
	codes := make([]int, customers)
	var wg sync.WaitGroup
	for i, b := range buyers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = f.apply(b.c, b.id, 2, "FIRST5").Code
		}()
	}
	wg.Wait()
	won, lost := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			won++
		case http.StatusConflict:
			lost++
		default:
			t.Errorf("an apply answered %d", code)
		}
	}
	if won != 5 || lost != customers-5 {
		t.Errorf("won %d, lost %d; want 5 and %d", won, lost, customers-5)
	}
	if f.used("FIRST5") != 5 || f.a.Count(`SELECT count(*) FROM coupon_redemptions`) != 5 || f.a.Count(`SELECT count(*) FROM orders WHERE coupon_id IS NOT NULL`) != 5 {
		t.Errorf("used %d, redemptions %d; want 5 each", f.used("FIRST5"), f.a.Count(`SELECT count(*) FROM coupon_redemptions`))
	}
}

// One customer with two orders applying the same single-use coupon twice at once: one gets it.
func TestCoupon_RaceOnePerCustomer(t *testing.T) {
	f := newFixture(t)
	f.makeCoupon(f.a.AsAdmin(), "ONCEONLY")
	first := f.awaiting()
	second := f.secondOrder()
	codes := make(chan int, 2)
	for _, id := range []uuid.UUID{first.o.ID, second} {
		go func() { codes <- f.apply(f.me, id, 2, "ONCEONLY").Code }()
	}
	got := []int{<-codes, <-codes}
	if !(got[0] == 200 && got[1] == 409 || got[0] == 409 && got[1] == 200) || f.used("ONCEONLY") != 1 {
		t.Errorf("answers %v, used %d; want one 200, one 409, one use", got, f.used("ONCEONLY"))
	}
}
