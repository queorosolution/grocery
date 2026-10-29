package security_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	apihttp "gp/internal/http"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

// seedOrder puts an order of the customer in the shop, as the database owner.
func (w world) seedOrder(customer, driver *uuid.UUID) uuid.UUID {
	w.a.T.Helper()
	id := uuid.New()
	code := "SEC-" + strings.ToUpper(id.String()[:6])
	w.a.Exec(`INSERT INTO orders (id, code, shop_id, customer_id, status, delivery_type, address, lat, lng, distance_m, weight_g,
		items_cents, delivery_fee_cents, total_cents, payment_provider, driver_id)
		VALUES ($1, $2, $3, $4, 'AWAITING_PAYMENT', 'COURIER', '{}', 6.93, 79.87, 1200, 1000, 100000, 15000, 115000, 'MANUAL', $5)`,
		id, code, w.shopA, customer, driver)
	return id
}

// bodyFor is a body the route accepts, so a refusal can only come from the
// ownership check and not from validation.
var bodyFor = map[string]string{
	"PATCH /me/addresses/{id}":              `{"label":"Work","line1":"1 Main Street, Colombo","lat":6.93,"lng":79.87}`,
	"POST /orders/{id}/accept-revision":     `{"version":1,"revisionNo":1}`,
	"POST /orders/{id}/cancel":              `{"version":1}`,
	"POST /orders/{id}/coupon":              `{"version":1,"code":"WELCOME10"}`,
	"DELETE /orders/{id}/coupon":            `{"version":1}`,
	"POST /orders/{id}/tickets":             `{"reason":"DAMAGED","note":"box was open"}`,
	"POST /orders/{id}/payment":             `{"version":1}`,
	"POST /driver/deliveries/{id}/fail":     `{"version":1,"reason":"nobody home"}`,
	"POST /driver/deliveries/{id}/complete": `{"version":1,"code":"1234"}`,
}

// AUTHZ-01, AUTHZ-03, DATA-05: every route that names an order or an address of
// the customer answers a stranger exactly as it answers an id that does not
// exist, and leaves the thing as it was. The owner is not turned away.
func TestAuthzMatrix_OtherPeoplesThingsAreNotFound(t *testing.T) {
	w := newWorld(t)
	a := w.a
	owner := w.who.customer
	stranger := a.SignedIn(fakes.Claims{Sub: "stranger", Email: "stranger@example.test"})
	orderID := w.seedOrder(&owner.Me.ID, nil)
	addr := apptest.Decode[struct{ Address struct{ ID uuid.UUID } }](t, a.As(owner).OK(http.MethodPost, "/me/addresses",
		`{"label":"Home","line1":"12 Galle Road, Colombo 03","lat":6.93,"lng":79.87}`)).Address.ID

	ids := map[string]uuid.UUID{"/me/addresses/{id}": addr, "/orders/{id}": orderID}
	idFor := func(pattern string) (uuid.UUID, bool) {
		for prefix, id := range ids {
			if strings.HasPrefix(pattern, prefix) || strings.HasPrefix(pattern, strings.TrimSuffix(prefix, "/{id}")+"/{id}") {
				return id, true
			}
		}
		return uuid.Nil, false
	}
	call := func(rt apihttp.Route, id uuid.UUID, s apptest.Session) *httptest.ResponseRecorder {
		path := "/v1" + strings.Replace(rt.Pattern, "{id}", id.String(), 1)
		return a.CallWith(apptest.DefaultIP, rt.Method, path, bodyFor[rt.Method+" "+rt.Pattern], map[string]string{"Idempotency-Key": "security-test-key-0002"}, s.Cookies()...)
	}

	checked := 0
	for _, rt := range apihttp.Routes(a.Deps) {
		if rt.Guard != apihttp.User || !strings.Contains(rt.Pattern, "{id}") {
			continue
		}
		id, ok := idFor(rt.Pattern)
		if !ok {
			t.Errorf("%s %s names an id, but this test does not know what it is", rt.Method, rt.Pattern)
			continue
		}
		checked++
		t.Run(rt.Method+" "+rt.Pattern, func(t *testing.T) {
			theirs, nothing := call(rt, id, stranger), call(rt, uuid.New(), stranger)
			if theirs.Code != http.StatusNotFound || theirs.Code != nothing.Code || errCode(theirs) != errCode(nothing) {
				t.Errorf("stranger on a real id: %d %s; on an unknown id: %d %s", theirs.Code, theirs.Body, nothing.Code, nothing.Body)
			}
		})
	}
	if checked < 8 {
		t.Fatalf("only %d routes with an {id} were checked", checked)
	}
	// The other side: the owner of a fresh thing is never answered "not found".
	for _, rt := range apihttp.Routes(a.Deps) {
		if rt.Guard != apihttp.User || !strings.Contains(rt.Pattern, "{id}") {
			continue
		}
		var id uuid.UUID
		if strings.HasPrefix(rt.Pattern, "/me/addresses") {
			id = apptest.Decode[struct{ Address struct{ ID uuid.UUID } }](t, a.As(owner).OK(http.MethodPost, "/me/addresses",
				`{"label":"Spare","line1":"3 Temple Road, Colombo 03","lat":6.93,"lng":79.87}`)).Address.ID
		} else {
			id = w.seedOrder(&owner.Me.ID, nil)
		}
		if got := call(rt, id, owner); got.Code == http.StatusNotFound {
			t.Errorf("%s %s: the owner got 404 (%s)", rt.Method, rt.Pattern, got.Body)
		}
	}
	// Nothing happened to the order, and the owner still reads it.
	var status string
	var version int
	if err := a.Env.Owner.QueryRow(t.Context(), `SELECT status, version FROM orders WHERE id = $1`, orderID).Scan(&status, &version); err != nil || status != "AWAITING_PAYMENT" || version != 1 {
		t.Errorf("the order is now %s v%d (%v)", status, version, err)
	}
	if got := a.As(owner).Call(http.MethodGet, "/orders/"+orderID.String(), ""); got.Code != http.StatusOK {
		t.Errorf("the owner reads their order: got %d", got.Code)
	}
	if got := a.As(stranger).Call(http.MethodGet, "/orders", ""); strings.Contains(got.Body.String(), orderID.String()) {
		t.Error("the stranger's list holds the order")
	}
}

func errCode(r *httptest.ResponseRecorder) string {
	var e struct {
		Error struct{ Code, Message string }
	}
	_ = json.Unmarshal(r.Body.Bytes(), &e)
	return e.Error.Code + "|" + e.Error.Message
}

// AUTHZ-03: a driver finishes only the orders given to them.
func TestAuthzMatrix_DriverOnlyTouchesOwnOrders(t *testing.T) {
	w := newWorld(t)
	a := w.a
	other := a.SignedIn(fakes.Claims{Sub: "driver-two", Email: "driver-two@example.test"})
	a.Grant(other.Me.ID, &w.shopA, string(enum.RoleShopDriver))
	order := w.seedOrder(&w.who.customer.Me.ID, &other.Me.ID) // belongs to the other driver
	for _, step := range []string{"complete", "fail"} {
		rt := apihttp.Route{Method: http.MethodPost, Pattern: "/driver/deliveries/{id}/" + step}
		path := "/v1" + strings.Replace(rt.Pattern, "{id}", order.String(), 1)
		got := a.Call(rt.Method, path, bodyFor[rt.Method+" "+rt.Pattern], w.who.driver.Cookies()...)
		if got.Code != http.StatusNotFound {
			t.Errorf("%s by another driver: got %d (%s)", step, got.Code, got.Body)
		}
	}
	var status string
	if err := a.Env.Owner.QueryRow(t.Context(), `SELECT status FROM orders WHERE id = $1`, order).Scan(&status); err != nil || status != "AWAITING_PAYMENT" {
		t.Errorf("the order is now %s (%v)", status, err)
	}
}
