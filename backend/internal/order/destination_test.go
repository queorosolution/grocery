package order_test

import (
	"net/http"
	"strings"
	"testing"
)

// destBody swaps the saved address in a request for a one-time destination.
func (f fixture) destBody(body, destination string) string {
	return strings.Replace(body, `"addressId":"`+f.addr.String()+`"`, `"destination":`+destination, 1)
}

const (
	farPlace  = `{"line1":"45 Temple Road, Kandy","landmark":"Opposite the bank","lat":7.2906,"lng":80.6337}`
	nearPlace = `{"label":" Mum's house ","line1":" 8 Lake Drive, Colombo 08 ","lat":6.93,"lng":79.87}`
)

// A one-time destination is priced and stored like a saved address, and never saved.
func TestDestination_SubmitToAnotherPlace(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	slot := f.firstSlot(f.shop)

	q := f.quote(f.destBody(f.body(`"slotId":"`+slot.String()+`"`, rice), nearPlace))
	if q.Totals.TotalCents != itemPrice+deliveryFee {
		t.Errorf("quote total = %d, want %d", q.Totals.TotalCents, itemPrice+deliveryFee)
	}

	o := f.placed(f.me, key1, f.destBody(f.body(`"slotId":"`+slot.String()+`"`, rice), nearPlace))
	if o.Address.Label != "Mum's house" || o.Address.Line1 != "8 Lake Drive, Colombo 08" {
		t.Errorf("order address = %+v, want the cleaned destination", o.Address)
	}
	if n := f.a.Count(`SELECT count(*) FROM addresses a JOIN users u ON u.id = a.user_id WHERE u.email = 'nimal@example.com'`); n != 1 {
		t.Errorf("saved addresses = %d, want only the first one", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND lat = 6.93 AND lng = 79.87`, o.ID); n != 1 {
		t.Error("the order did not keep the destination pin")
	}
}

// The distance rule works from the destination, not from the saved address.
func TestDestination_DistanceRuleApplies(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	body := f.destBody(f.body(`"slotId":"`+f.firstSlot(f.shop).String()+`"`, rice), farPlace)
	if q := f.quote(body); q.Checks.InRange {
		t.Error("quote says a place 94 km away is in range")
	}
	if w := f.submit(f.me, key1, body); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "OUT_OF_RANGE") {
		t.Errorf("POST /bills = %d %s, want 409 OUT_OF_RANGE", w.Code, w.Body)
	}
}

func TestDestination_BadRequests(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	body := f.body("", rice)

	// Both, or neither, of the address and the destination.
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(body, `"lines"`, `"destination":`+nearPlace+`,"lines"`, 1), http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(body, `"addressId":"`+f.addr.String()+`",`, "", 1), http.StatusBadRequest, "VALIDATION_ERROR")
	// A destination is held to the same rules as an address.
	f.me.Expect(http.MethodPost, "/bills/quote", f.destBody(body, `{"line1":"x","lat":6.93,"lng":79.87}`), http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/bills/quote", f.destBody(body, `{"line1":"8 Lake Drive","lat":99,"lng":79.87}`), http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/bills/quote", f.destBody(body, `{"line1":"8 Lake Drive","lat":6.93,"lng":79.87,"isDefault":true}`), http.StatusBadRequest, "VALIDATION_ERROR")
}
