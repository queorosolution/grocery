package order_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"gp/test/apptest"
	"gp/test/fakes"
)

func TestSubmit_NeedsAGoodKey(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	body := f.body(`"slotId":"`+f.firstSlot(f.shop).String()+`"`, rice)
	for _, key := range []string{"", "short", "has spaces in the key!", "x/../../etc/passwd-0000000"} {
		w := f.submit(f.me, key, body)
		if w.Code != http.StatusBadRequest || apptest.ErrorOf(t, w).Error.Fields["Idempotency-Key"] == "" {
			t.Errorf("key %q = %d %s, want 400 with a field", key, w.Code, w.Body)
		}
	}
	if f.a.Count(`SELECT count(*) FROM orders`) != 0 {
		t.Error("a bill without a good key made an order")
	}
}

// BIZ-05: the same key twice is one order; the same key with another body is refused.
func TestSubmit_SameKeyIsOneOrder(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	slot := f.firstSlot(f.shop)
	body := f.body(`"slotId":"`+slot.String()+`"`, rice)

	first := f.submit(f.me, key1, body)
	again := f.submit(f.me, key1, body) // the cart is gone by now; the retry still answers
	if first.Code != http.StatusCreated || again.Code != http.StatusCreated || !sameJSON(t, first.Body.Bytes(), again.Body.Bytes()) {
		t.Fatalf("first %d %s, again %d %s; want the same 201", first.Code, first.Body, again.Code, again.Body)
	}
	if f.a.Count(`SELECT count(*) FROM orders`) != 1 || f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot) != 1 {
		t.Error("a retry made a second order or took a second place")
	}
	other := f.body(`"slotId":"`+slot.String()+`","note":"different"`, rice)
	w := f.submit(f.me, key1, other)
	if w.Code != http.StatusConflict || apptest.ErrorOf(t, w).Error.Code != "IDEMPOTENCY_MISMATCH" {
		t.Errorf("same key, other body = %d %s, want 409 IDEMPOTENCY_MISMATCH", w.Code, w.Body)
	}
	// A key belongs to one customer: another customer with the same key is a new bill.
	stranger := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "kamal", Email: "kamal@example.com", Name: "Kamal"}))
	f.verify("kamal@example.com")
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	theirs := f.address(stranger)
	if w := f.submit(stranger, key1, f.bodyOf(cartID, theirs, `"slotId":"`+slot.String()+`"`, rice)); w.Code != http.StatusCreated {
		t.Errorf("another customer's bill with the same key = %d %s", w.Code, w.Body)
	}
}

func TestSubmit_ParallelRetriesAreOneOrder(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	slot := f.firstSlot(f.shop)
	body := f.body(`"slotId":"`+slot.String()+`"`, rice)

	var wg sync.WaitGroup
	bodies := make([]string, 8)
	codes := make([]int, 8)
	for i := range bodies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := f.submit(f.me, key1, body)
			codes[i], bodies[i] = w.Code, w.Body.String()
		}()
	}
	wg.Wait()
	for i := range bodies {
		if codes[i] != http.StatusCreated || !sameJSON(t, []byte(bodies[i]), []byte(bodies[0])) {
			t.Errorf("call %d = %d %s", i, codes[i], bodies[i])
		}
	}
	if f.a.Count(`SELECT count(*) FROM orders`) != 1 || f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot) != 1 {
		t.Error("parallel retries made more than one order")
	}
}

// BIZ-12: three open unpaid orders at most, and ten bills an hour.
func TestSubmit_OrderFlooding(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	slot := f.slot(f.shop, 50)
	send := func(i int) (code int, err string) {
		f.cart(f.tenant, cartLine{rice, 1, itemPrice})
		w := f.submit(f.me, fmt.Sprintf("flood-key-%06d", i), f.body(`"slotId":"`+slot.String()+`"`, rice))
		if w.Code >= 300 {
			err = apptest.ErrorOf(t, w).Error.Code
		}
		return w.Code, err
	}
	for i := 1; i <= 3; i++ {
		if code, e := send(i); code != http.StatusCreated {
			t.Fatalf("order %d = %d %s", i, code, e)
		}
	}
	if code, e := send(4); code != http.StatusConflict || e != "TOO_MANY_OPEN_ORDERS" {
		t.Fatalf("4th open order = %d %s, want 409 TOO_MANY_OPEN_ORDERS", code, e)
	}
	// Once the others are no longer open, bills go through until the hour is used up (attempts count, 10 an hour).
	f.a.Exec(`UPDATE orders SET status = 'DELIVERED'`)
	for i := 5; i <= 10; i++ {
		if code, e := send(i); code != http.StatusCreated {
			t.Fatalf("bill %d = %d %s", i, code, e)
		}
		f.a.Exec(`UPDATE orders SET status = 'DELIVERED'`)
	}
	if code, e := send(11); code != http.StatusTooManyRequests || e != "RATE_LIMITED" {
		t.Errorf("11th bill in the hour = %d %s, want 429 RATE_LIMITED", code, e)
	}
}

// BIZ-04: twenty customers at once for a slot with five places make exactly five orders.
func TestSubmit_SlotIsNeverOverbooked(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.slot(f.shop, 50) // keeps the shop ready when the small slot fills
	slot := f.slot(f.shop, 5)

	const n = 20
	type buyer struct {
		c    apptest.Client
		body string
		key  string
	}
	buyers := make([]buyer, n)
	for i := range buyers {
		email := fmt.Sprintf("buyer%d@example.com", i)
		c := f.a.As(f.a.SignedIn(fakes.Claims{Sub: fmt.Sprintf("buyer-%d", i), Email: email, Name: "Buyer"}))
		f.verify(email)
		cart := fmt.Sprintf("crt_buyer%d", i)
		f.a.Cart.PutCart(f.tenant, cartOf(cart, rice))
		buyers[i] = buyer{c, f.bodyOf(cart, f.address(c), `"slotId":"`+slot.String()+`"`, rice), fmt.Sprintf("buyer-key-%08d", i)}
	}
	var wg sync.WaitGroup
	codes := make([]string, n)
	for i, b := range buyers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := f.submit(b.c, b.key, b.body)
			codes[i] = fmt.Sprint(w.Code)
			if w.Code >= 300 {
				codes[i] = apptest.ErrorOf(t, w).Error.Code
			}
		}()
	}
	wg.Wait()
	made, full := 0, 0
	for _, c := range codes {
		switch c {
		case "201":
			made++
		case "SLOT_FULL":
			full++
		}
	}
	if made != 5 || full != n-5 || f.a.Count(`SELECT count(*) FROM orders WHERE slot_id = $1`, slot) != 5 || f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot) != 5 {
		t.Errorf("made %d, full %d (%v); want exactly 5 orders", made, full, codes)
	}
}

// sameJSON compares two bodies as data: the stored copy of an answer keeps its
// fields in the database's order, not the first answer's.
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
