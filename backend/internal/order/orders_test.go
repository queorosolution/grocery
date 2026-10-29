package order_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"gp/internal/order"
	"gp/test/apptest"
	"gp/test/fakes"
)

// AUTHZ-01: a customer reads only their own orders; everything else is "not found".
func TestOrders_OnlyMine(t *testing.T) {
	f := newFixture(t)
	o, _ := f.submitted()
	stranger := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "kamal", Email: "kamal@example.com", Name: "Kamal"}))

	stranger.Expect(http.MethodGet, "/orders/"+o.ID.String(), "", http.StatusNotFound, "NOT_FOUND")
	stranger.Expect(http.MethodGet, "/orders/00000000-0000-4000-8000-000000000009", "", http.StatusNotFound, "NOT_FOUND")
	stranger.Expect(http.MethodGet, "/orders/not-an-id", "", http.StatusNotFound, "NOT_FOUND")
	if got := apptest.Decode[order.SummaryPage](t, stranger.OK(http.MethodGet, "/orders", "")); len(got.Data) != 0 || got.NextCursor != nil {
		t.Errorf("the stranger's list = %+v, want empty", got)
	}
	if w := f.a.Call(http.MethodGet, "/v1/orders/"+o.ID.String(), ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no login = %d, want 401", w.Code)
	}
	if w := f.a.Call(http.MethodGet, "/v1/orders", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("list with no login = %d, want 401", w.Code)
	}
	got := f.get(f.me, o.ID)
	if got.ID != o.ID || got.Customer != nil || got.Actions == nil {
		t.Errorf("own order = %+v", got)
	}
}

func TestOrders_ListPages(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	slot := f.slot(f.shop, 50)
	var ids []string
	for i := 1; i <= 3; i++ {
		f.cart(f.tenant, cartLine{rice, i, itemPrice})
		o := f.placed(f.me, fmt.Sprintf("list-key-%07d", i), f.body(`"slotId":"`+slot.String()+`"`, rice))
		ids = append([]string{o.ID.String()}, ids...) // newest first
		f.a.Exec(`UPDATE orders SET status = 'DELIVERED'`)
	}
	first := apptest.Decode[order.SummaryPage](t, f.me.OK(http.MethodGet, "/orders?limit=2", ""))
	if len(first.Data) != 2 || first.NextCursor == nil || first.Data[0].ID.String() != ids[0] || first.Data[1].ID.String() != ids[1] {
		t.Fatalf("page 1 = %+v, want the two newest and a cursor", first)
	}
	if first.Data[0].ItemCount != 1 || first.Data[0].Shop.Slug != "krishna" || first.Data[0].TotalCents != 3*itemPrice+deliveryFee {
		t.Errorf("summary = %+v", first.Data[0])
	}
	second := apptest.Decode[order.SummaryPage](t, f.me.OK(http.MethodGet, "/orders?limit=2&cursor="+url.QueryEscape(*first.NextCursor), ""))
	if len(second.Data) != 1 || second.NextCursor != nil || second.Data[0].ID.String() != ids[2] {
		t.Errorf("page 2 = %+v, want the oldest and no cursor", second)
	}
	f.me.Expect(http.MethodGet, "/orders?cursor=garbage", "", http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodGet, "/orders?limit=abc", "", http.StatusBadRequest, "VALIDATION_ERROR")
	if all := apptest.Decode[order.SummaryPage](t, f.me.OK(http.MethodGet, "/orders?limit=500", "")); len(all.Data) != 3 {
		t.Errorf("a big limit returned %d orders, want 3", len(all.Data))
	}
}
