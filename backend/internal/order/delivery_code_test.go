package order_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

func driverPath(id uuid.UUID, rest string) string { return "/driver/deliveries/" + id.String() + rest }

func (f fixture) code(id uuid.UUID) string {
	f.a.T.Helper()
	d := f.get(f.me, id)
	if d.DeliveryCode == nil {
		f.a.T.Fatal("the customer sees no delivery code")
	}
	return *d.DeliveryCode
}

func (f fixture) complete(c apptest.Client, id uuid.UUID, code string) *http.Response {
	f.a.T.Helper()
	v := f.get(f.me, id).Version
	return c.Call(http.MethodPost, driverPath(id, "/complete"), version(v, `"code":"`+code+`"`)).Result()
}

// dayOf is the ?day= query that lists the order's slot (the slot is tomorrow).
func (f fixture) dayOf(id uuid.UUID) string {
	f.a.T.Helper()
	var d string
	err := f.a.Env.Owner.QueryRow(f.a.T.Context(), `SELECT to_char(sl.starts_at AT TIME ZONE 'Asia/Colombo', 'YYYY-MM-DD')
		FROM orders o JOIN slots sl ON sl.id = o.slot_id WHERE o.id = $1`, id).Scan(&d)
	if err != nil {
		f.a.T.Fatal(err)
	}
	return "?day=" + d
}

func wrongCode(code string) string {
	if code == "0000" {
		return "1111"
	}
	return "0000"
}

// DATA-04: only the customer sees the code, and only while the order is out.
func TestDeliveryCode_OnlyTheCustomerSees(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	if f.get(f.me, s.id).DeliveryCode != nil {
		t.Error("the code is shown before the order is out for delivery")
	}
	f.move(s.staff, s.id, "PICKING")
	f.move(s.staff, s.id, "PACKED")
	f.assign(s.staff, f.consolePath(f.shop, "/"+s.id.String()+"/driver"), s.id, s.driverID)
	f.move(s.staff, s.id, "OUT_FOR_DELIVERY")
	code := f.code(s.id)
	if len(code) != 4 || strings.Trim(code, "0123456789") != "" {
		t.Fatalf("code = %q, want 4 digits", code)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND delivery_code_enc IS NOT NULL AND position(convert_to($2, 'UTF8') in delivery_code_enc) = 0`, s.id, code) != 1 {
		t.Error("the code is stored as plain text")
	}
	for name, raw := range map[string]string{
		"console": s.staff.OK(http.MethodGet, f.consolePath(f.shop, "/"+s.id.String()), "").Body.String(),
		"driver":  s.driver.OK(http.MethodGet, "/driver/deliveries"+f.dayOf(s.id), "").Body.String(),
	} {
		if strings.Contains(raw, "deliveryCode") || strings.Contains(raw, `"`+code+`"`) {
			t.Errorf("%s view leaks the delivery code", name)
		}
	}
}

// BIZ-11: five wrong tries are allowed, the sixth is locked even with the right code.
func TestDeliveryCode_LocksAfterFiveTries(t *testing.T) {
	f := newFixture(t)
	s := f.outForDelivery()
	code := f.code(s.id)
	for i := 1; i <= 5; i++ {
		v := f.get(f.me, s.id).Version
		s.driver.Expect(http.MethodPost, driverPath(s.id, "/complete"), version(v, `"code":"`+wrongCode(code)+`"`), http.StatusBadRequest, "DELIVERY_CODE_INVALID")
	}
	if f.a.Count(`SELECT delivery_code_attempts FROM orders WHERE id = $1`, s.id) != 5 {
		t.Fatal("wrong tries were not counted")
	}
	v := f.get(f.me, s.id).Version
	s.driver.Expect(http.MethodPost, driverPath(s.id, "/complete"), version(v, `"code":"`+code+`"`), http.StatusLocked, "DELIVERY_CODE_LOCKED")
	if f.status(s.id) != "OUT_FOR_DELIVERY" {
		t.Fatal("a locked order was delivered")
	}

	// Only the admin unlocks it, and it is audited.
	s.staff.Expect(http.MethodPost, "/admin/orders/"+s.id.String()+"/unlock-code", "", http.StatusForbidden, "FORBIDDEN")
	f.admin().OK(http.MethodPost, "/admin/orders/"+s.id.String()+"/unlock-code", "")
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'order.unlock_code'`) != 1 {
		t.Error("the unlock is not audited")
	}
	if r := f.complete(s.driver, s.id, code); r.StatusCode != http.StatusOK {
		t.Fatalf("complete after unlock = %d", r.StatusCode)
	}
}

func TestDeliveryCode_Validation(t *testing.T) {
	f := newFixture(t)
	s := f.outForDelivery()
	for _, bad := range []string{"", "12", "12345", "abcd", "12 4"} {
		v := f.get(f.me, s.id).Version
		s.driver.Expect(http.MethodPost, driverPath(s.id, "/complete"), version(v, fmt.Sprintf(`"code":%q`, bad)), http.StatusBadRequest, "VALIDATION_ERROR")
	}
	if f.a.Count(`SELECT delivery_code_attempts FROM orders WHERE id = $1`, s.id) != 0 {
		t.Error("a malformed code counted as a try")
	}
	s.driver.Expect(http.MethodPost, driverPath(s.id, "/complete"), version(99, `"code":"1234"`), http.StatusConflict, "STALE_ORDER")
}

// The full delivery: the ledger adds up to the total with the delivery fee, and the shop's
// own driver means the fee is the shop's.
func TestDeliveryCode_DeliversAndPostsLedger(t *testing.T) {
	f := newFixture(t)
	s := f.outForDelivery()
	got := apptest.Decode[order.Detail](t, s.driver.OK(http.MethodPost, driverPath(s.id, "/complete"),
		version(f.get(f.me, s.id).Version, `"code":"`+f.code(s.id)+`"`)))
	if got.Status != enum.OrderStatusDelivered || got.DeliveryCode != nil {
		t.Fatalf("delivered = %s, code %v", got.Status, got.DeliveryCode)
	}
	var sum int64
	fee := ledgerRow{}
	for _, r := range f.ledgerOf(s.id) {
		sum += r.Amount
		if r.Type == "DELIVERY_FEE" {
			fee = r
		}
	}
	if want := int64(f.a.Count(`SELECT total_cents FROM orders WHERE id = $1`, s.id)); sum != want {
		t.Errorf("ledger adds up to %d, want the total %d", sum, want)
	}
	if fee.Party != "SHOP" || fee.Amount != deliveryFee {
		t.Errorf("DELIVERY_FEE = %+v, want SHOP %d", fee, deliveryFee)
	}
	if f.events("order.delivered", s.id) != 1 || f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND delivered_at IS NOT NULL AND closed_at IS NOT NULL`, s.id) != 1 {
		t.Error("the delivery did not close the order and emit its event")
	}
	// A second try finds nothing to deliver.
	s.driver.Expect(http.MethodPost, driverPath(s.id, "/complete"), version(got.Version, `"code":"1234"`), http.StatusConflict, "INVALID_TRANSITION")
}

// AUTHZ-03: a driver cannot see or finish another driver's order.
func TestDeliveryCode_OtherDriverGetsNotFound(t *testing.T) {
	f := newFixture(t)
	s := f.outForDelivery()
	code := f.code(s.id)
	other := f.staff("nuwan", string(enum.RoleShopDriver))
	v := f.get(f.me, s.id).Version
	other.Expect(http.MethodPost, driverPath(s.id, "/complete"), version(v, `"code":"`+code+`"`), http.StatusNotFound, "NOT_FOUND")
	other.Expect(http.MethodPost, driverPath(s.id, "/fail"), version(v, `"reason":"not mine"`), http.StatusNotFound, "NOT_FOUND")
	if n := len(apptest.Decode[struct {
		Data []order.DeliveryGroup `json:"data"`
	}](t, other.OK(http.MethodGet, "/driver/deliveries"+f.dayOf(s.id), "")).Data); n != 0 {
		t.Errorf("another driver lists %d groups", n)
	}
	if f.status(s.id) != "OUT_FOR_DELIVERY" || f.a.Count(`SELECT delivery_code_attempts FROM orders WHERE id = $1`, s.id) != 0 {
		t.Error("another driver changed the order")
	}
	// The customer, with no driver role, has no driver routes at all.
	f.me.Expect(http.MethodPost, driverPath(s.id, "/complete"), version(v, `"code":"`+code+`"`), http.StatusForbidden, "FORBIDDEN")
}

func TestDelivery_FailAndRetry(t *testing.T) {
	f := newFixture(t)
	s := f.outForDelivery()
	v := f.get(f.me, s.id).Version
	s.driver.Expect(http.MethodPost, driverPath(s.id, "/fail"), version(v, `"reason":"x"`), http.StatusBadRequest, "VALIDATION_ERROR")
	s.driver.OK(http.MethodPost, driverPath(s.id, "/fail"), version(v, `"reason":"Customer not at home"`))
	if f.status(s.id) != "DELIVERY_FAILED" {
		t.Fatalf("status = %s", f.status(s.id))
	}
	// The shop sends it out again (T18) with a fresh code.
	f.move(s.staff, s.id, "OUT_FOR_DELIVERY")
	if r := f.complete(s.driver, s.id, f.code(s.id)); r.StatusCode != http.StatusOK {
		t.Fatalf("complete after retry = %d", r.StatusCode)
	}
}

func TestDriver_DeliveriesAndStats(t *testing.T) {
	f := newFixture(t)
	s := f.outForDelivery()
	groups := apptest.Decode[struct {
		Data []order.DeliveryGroup `json:"data"`
	}](t, s.driver.OK(http.MethodGet, "/driver/deliveries"+f.dayOf(s.id), "")).Data
	if len(groups) == 0 || len(groups[0].Orders) != 1 || groups[0].Orders[0].ID != s.id {
		t.Fatalf("deliveries = %+v", groups)
	}
	o := groups[0].Orders[0]
	if o.Status != enum.OrderStatusOutForDelivery || o.Customer.Phone == "" || o.Address.Line1 == "" {
		t.Errorf("driver order = %+v", o)
	}
	s.driver.Expect(http.MethodGet, "/driver/deliveries?day=junk", "", http.StatusBadRequest, "VALIDATION_ERROR")

	s.driver.OK(http.MethodPost, driverPath(s.id, "/complete"), version(f.get(f.me, s.id).Version, `"code":"`+f.code(s.id)+`"`))
	stats := apptest.Decode[order.DriverStats](t, s.driver.OK(http.MethodGet, "/driver/stats", ""))
	if stats.Delivered != 1 || stats.Failed != 0 {
		t.Errorf("stats = %+v", stats)
	}
	s.driver.Expect(http.MethodGet, "/driver/stats?from=2030-01-02&to=2030-01-01", "", http.StatusBadRequest, "VALIDATION_ERROR")
	// Stats are the caller's own.
	other := f.staff("nuwan", string(enum.RoleShopDriver))
	if st := apptest.Decode[order.DriverStats](t, other.OK(http.MethodGet, "/driver/stats", "")); st.Delivered != 0 {
		t.Errorf("another driver's stats = %+v", st)
	}
}

// The buy-again list counts what was delivered, once per product per order.
func TestBuyAgain_CountsDeliveredLines(t *testing.T) {
	f := newFixture(t)
	s := f.outForDelivery()
	s.driver.OK(http.MethodPost, driverPath(s.id, "/complete"), version(f.get(f.me, s.id).Version, `"code":"`+f.code(s.id)+`"`))
	for range 2 { // a second drain must not count the order again
		if err := f.a.DrainEvents(); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.a.Count(`SELECT count(*) FROM customer_products WHERE user_id = $1 AND shop_id = $2 AND times = 1`, f.me.Session.Me.ID, f.shop); n != 2 {
		t.Errorf("buy-again rows = %d, want the 2 delivered products", n)
	}
}
