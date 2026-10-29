package order_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

// shopFlow is a paid order, the shop's staff and a shop driver, ready to be picked.
type shopFlow struct {
	x             trio
	id            uuid.UUID
	staff, driver apptest.Client
	driverID      uuid.UUID
}

func (f fixture) paidFlow() shopFlow {
	f.a.T.Helper()
	x := f.unpaid()
	f.paid(x)
	d := f.staff("kamal", string(enum.RoleShopDriver))
	return shopFlow{x: x, id: x.o.ID, staff: f.staff("ravi", string(enum.RoleShopStaff)), driver: d, driverID: d.Session.Me.ID}
}

func (f fixture) move(c apptest.Client, id uuid.UUID, to string) order.Detail {
	f.a.T.Helper()
	v := f.get(f.me, id).Version
	w := c.Call(http.MethodPost, f.consolePath(f.shop, "/"+id.String()+"/status"), version(v, `"to":"`+to+`"`))
	if w.Code != http.StatusOK {
		f.a.T.Fatalf("status %s = %d %s", to, w.Code, w.Body)
	}
	return apptest.Decode[order.Detail](f.a.T, w)
}

func (f fixture) assign(c apptest.Client, path string, id, driver uuid.UUID) *http.Response {
	f.a.T.Helper()
	v := f.get(f.me, id).Version
	return c.Call(http.MethodPost, path, version(v, `"driverId":"`+driver.String()+`"`)).Result()
}

// outForDelivery takes the paid order through picking and packing, picks the shop driver and sends it out.
func (f fixture) outForDelivery() shopFlow {
	f.a.T.Helper()
	s := f.paidFlow()
	f.move(s.staff, s.id, "PICKING")
	f.move(s.staff, s.id, "PACKED")
	path := f.consolePath(f.shop, "/"+s.id.String()+"/driver")
	if r := f.assign(s.staff, path, s.id, s.driverID); r.StatusCode != http.StatusOK {
		f.a.T.Fatalf("assign = %d", r.StatusCode)
	}
	f.move(s.staff, s.id, "OUT_FOR_DELIVERY")
	return s
}

func TestFulfil_ShopFlow(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	if got := f.move(s.staff, s.id, "PICKING"); got.Status != enum.OrderStatusPicking {
		t.Fatalf("picking = %s", got.Status)
	}
	f.move(s.staff, s.id, "PACKED")

	// Out for delivery needs a driver (T15).
	v := f.get(f.me, s.id).Version
	s.staff.Expect(http.MethodPost, f.consolePath(f.shop, "/"+s.id.String()+"/status"), version(v, `"to":"OUT_FOR_DELIVERY"`), http.StatusBadRequest, "VALIDATION_ERROR")
	if f.status(s.id) != "PACKED" {
		t.Fatal("an order with no driver was sent out")
	}
	path := f.consolePath(f.shop, "/"+s.id.String()+"/driver")
	if r := f.assign(s.staff, path, s.id, s.driverID); r.StatusCode != http.StatusOK {
		t.Fatalf("assign = %d", r.StatusCode)
	}
	if got := f.move(s.staff, s.id, "OUT_FOR_DELIVERY"); got.Status != enum.OrderStatusOutForDelivery {
		t.Fatalf("out = %s", got.Status)
	}
	if f.events("order.out_for_delivery", s.id) != 1 {
		t.Error("no out-for-delivery event")
	}
	// A target outside the shop's side of the flow is a bad request.
	v = f.get(f.me, s.id).Version
	s.staff.Expect(http.MethodPost, f.consolePath(f.shop, "/"+s.id.String()+"/status"), version(v, `"to":"DELIVERED"`), http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestFulfil_DriversList(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	f.a.Grant(f.a.SignedIn(claims("pf")).Me.ID, nil, string(enum.RolePlatformDriver))
	type driverList struct {
		Data []struct {
			ID uuid.UUID `json:"id"`
		} `json:"data"`
	}
	list := apptest.Decode[driverList](t, s.staff.OK(http.MethodGet, "/console/shops/"+f.shop.String()+"/drivers", ""))
	if len(list.Data) != 1 || list.Data[0].ID != s.driverID {
		t.Errorf("drivers = %+v, want only the shop driver", list.Data)
	}
	// The driver has no business in the shop's list.
	s.driver.Expect(http.MethodGet, "/console/shops/"+f.shop.String()+"/drivers", "", http.StatusForbidden, "FORBIDDEN")
}

func TestFulfil_DriverSource(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	pf := f.a.As(f.a.SignedIn(claims("pf")))
	f.a.Grant(pf.Session.Me.ID, nil, string(enum.RolePlatformDriver))
	shopPath := f.consolePath(f.shop, "/"+s.id.String()+"/driver")
	adminPath := "/admin/orders/" + s.id.String() + "/driver"

	// Staff never pick a platform driver, whatever the source is.
	f.a.Exec(`UPDATE shop_settings SET driver_source = 'BOTH' WHERE shop_id = $1`, f.shop)
	if r := f.assign(s.staff, shopPath, s.id, pf.Session.Me.ID); r.StatusCode != http.StatusBadRequest {
		t.Errorf("staff picked a platform driver: %d", r.StatusCode)
	}
	// A shop with its own drivers only: the admin cannot add a platform driver either.
	f.a.Exec(`UPDATE shop_settings SET driver_source = 'SHOP' WHERE shop_id = $1`, f.shop)
	if r := f.assign(f.admin(), adminPath, s.id, pf.Session.Me.ID); r.StatusCode != http.StatusBadRequest {
		t.Errorf("platform driver on a SHOP-source shop: %d", r.StatusCode)
	}
	// PLATFORM source: the shop's own driver is not allowed, a platform one is.
	f.a.Exec(`UPDATE shop_settings SET driver_source = 'PLATFORM' WHERE shop_id = $1`, f.shop)
	if r := f.assign(s.staff, shopPath, s.id, s.driverID); r.StatusCode != http.StatusBadRequest {
		t.Errorf("shop driver on a PLATFORM-source shop: %d", r.StatusCode)
	}
	if r := f.assign(f.admin(), adminPath, s.id, pf.Session.Me.ID); r.StatusCode != http.StatusOK {
		t.Fatalf("admin assign = %d", r.StatusCode)
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'order.assign_driver'`) != 1 {
		t.Error("the admin's assignment is not audited")
	}
	// A user who is not a driver at all is refused.
	if r := f.assign(f.admin(), adminPath, s.id, f.me.Session.Me.ID); r.StatusCode != http.StatusBadRequest {
		t.Errorf("a customer as driver: %d", r.StatusCode)
	}
	// The staff route needs the permission: a driver cannot assign.
	if r := f.assign(s.driver, shopPath, s.id, s.driverID); r.StatusCode != http.StatusForbidden {
		t.Errorf("driver assigns: %d", r.StatusCode)
	}
}

func TestFulfil_StaleVersionAndOtherShop(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	s.staff.Expect(http.MethodPost, f.consolePath(f.shop, "/"+s.id.String()+"/status"), version(9, `"to":"PICKING"`), http.StatusConflict, "STALE_ORDER")
	other, _ := f.readyShop("other")
	// The same order through another shop's console is not found.
	stranger := f.a.As(f.a.SignedIn(claims("stranger")))
	f.a.Grant(stranger.Session.Me.ID, &other, string(enum.RoleShopStaff))
	stranger.Expect(http.MethodPost, "/console/shops/"+other.String()+"/orders/"+s.id.String()+"/status", version(1, `"to":"PICKING"`), http.StatusNotFound, "NOT_FOUND")
	if !strings.EqualFold(f.status(s.id), "PAID") {
		t.Error("another shop moved the order")
	}
}

func claims(sub string) fakes.Claims {
	return fakes.Claims{Sub: sub, Email: sub + "@example.com", Name: sub}
}
