package order

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

func sampleOrder(status enum.OrderStatus) orderData {
	price, line := money.Cents(102500), money.Cents(205000)
	driver := uuid.New()
	slot := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	return orderData{
		orderRow: orderRow{ID: uuid.New(), Code: "KR-7Q2M9X", ShopID: uuid.New(), CustomerID: uuid.New(), DriverID: &driver, Status: status, Version: 3, RevisionNo: 1},
		Address:  addressSnapshot{Label: "Home", Line1: "12 Galle Road", Phone: "+94771234567"},
		Lat:      6.93, Lng: 79.87,
		Shop:       ShopRef{ID: uuid.New(), Slug: "krishna", Name: "Krishna"},
		Slot:       &SlotRef{StartsAt: slot, EndsAt: slot.Add(2 * time.Hour)},
		CustomerNm: "Nimal",
		Totals:     billing.Totals{ItemsCents: 205000, DeliveryFeeCents: 15000, TotalCents: 220000},
		Coupon:     &CouponRef{Code: "WELCOME10"},
		Payment:    PaymentRef{Provider: enum.PaymentProviderPayhere},
		Items: []LineView{
			{ID: uuid.New(), Name: "Rice", Unit: "PCS", Qty: 2, UnitPriceCents: &price, LineCents: &line, State: "ACTIVE", SubPref: enum.SubstitutionPrefSimilar, RevisionNo: 0},
			{ID: uuid.New(), Name: "Rice 2", Unit: "PCS", Qty: 2, UnitPriceCents: &price, LineCents: &line, State: "ACTIVE", SubPref: enum.SubstitutionPrefSimilar, RevisionNo: 1},
		},
		Steps: []Step{{Status: enum.OrderStatusSubmitted, At: slot}},
	}
}

func keys(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// AUTHZ-06: each viewer sees its own fields and nobody else's.
func TestView_FieldsPerViewer(t *testing.T) {
	d := sampleOrder(enum.OrderStatusOutForDelivery)
	driverScope := db.Scope{UserID: *d.DriverID, Role: enum.RoleShopDriver}
	ownerScope := db.Scope{UserID: uuid.New(), ShopID: d.ShopID, Role: enum.RoleShopOwner}
	customerScope := db.Scope{UserID: d.CustomerID, Role: enum.RoleCustomer}

	driver := keys(t, d.Detail(ViewDriver, driverScope))
	for _, k := range []string{"totals", "coupon", "payment", "deliveryCode", "previousLines", "timeline", "expiresAt"} {
		if _, has := driver[k]; has {
			t.Errorf("the driver view has %q", k)
		}
	}
	for _, k := range []string{"totalCents", "customer", "address", "lines", "allowedActions"} {
		if _, has := driver[k]; !has {
			t.Errorf("the driver view lacks %q", k)
		}
	}
	var lines []map[string]any
	_ = json.Unmarshal(driver["lines"], &lines)
	for _, l := range lines {
		if _, has := l["unitPriceCents"]; has {
			t.Errorf("a driver line has a price: %v", l)
		}
		if _, has := l["lineCents"]; has {
			t.Errorf("a driver line has a line total: %v", l)
		}
	}
	if string(driver["address"]) == "" || !contains(driver["address"], `"lat":6.93`) {
		t.Errorf("the driver needs the pin: %s", driver["address"])
	}

	customer := keys(t, d.Detail(ViewCustomer, customerScope))
	if _, has := customer["customer"]; has {
		t.Error("the customer view carries a customer block")
	}
	if contains(customer["address"], `"lat"`) {
		t.Errorf("the customer view carries the pin: %s", customer["address"])
	}
	for _, k := range []string{"totals", "coupon", "payment", "timeline", "previousLines"} {
		if _, has := customer[k]; !has {
			t.Errorf("the customer view lacks %q", k)
		}
	}

	console := keys(t, d.Detail(ViewConsole, ownerScope))
	if !contains(console["customer"], `"phoneMasked":"+94******567"`) || contains(console["customer"], `"phone":`) {
		t.Errorf("console customer outside picking = %s, want the masked phone only", console["customer"])
	}
	if _, has := console["deliveryCode"]; has {
		t.Error("the console view has a delivery code")
	}
	picking := sampleOrder(enum.OrderStatusPicking)
	if !contains(keys(t, picking.Detail(ViewConsole, ownerScope))["customer"], `"phone":"+94771234567"`) {
		t.Error("the full phone is needed while picking")
	}
}

func contains(raw json.RawMessage, s string) bool { return strings.Contains(string(raw), s) }

func TestView_SplitsRevisions(t *testing.T) {
	d := sampleOrder(enum.OrderStatusRevised)
	cur, prev := d.split()
	if len(cur) != 1 || len(prev) != 1 || cur[0].RevisionNo != 1 || prev[0].RevisionNo != 0 {
		t.Errorf("split = %+v / %+v", cur, prev)
	}
}

func TestMaskPhone(t *testing.T) {
	for in, want := range map[string]string{"+94771234567": "+94******567", "123": "***", "": ""} {
		if got := maskPhone(in); got != want {
			t.Errorf("maskPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEventFor_EveryStatusHasAValidEvent(t *testing.T) {
	for _, s := range enum.OrderStatusValues() {
		if !eventFor(s).Valid() {
			t.Errorf("no event for %s", s)
		}
	}
}
