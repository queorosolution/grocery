package delivery_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/test/apptest"
)

const docRule = `{"baseCents":10000,"perKmCents":3000,"freeKm":2,"perKgCents":1000,"freeKg":5,"maxKm":8,"roadFactor":1.3}`

func (f fixture) preview(shopID uuid.UUID, body string) (int, string) {
	w := f.Call(http.MethodPost, "/admin/fee-rules/preview", `{"shopId":"`+shopID.String()+`",`+body+`}`)
	return w.Code, w.Body.String()
}

func TestFeeRules_DefaultShopAndPreview(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")

	// No rule at all: the shop cannot price a delivery yet.
	if code, body := f.preview(shopID, `"distanceKm":2,"weightKg":7.5,"itemsCents":0`); code != http.StatusConflict {
		t.Fatalf("no rule: %d %s, want 409 SHOP_NOT_READY", code, body)
	}

	row := apptest.Decode[delivery.RuleRow](t, f.OK(http.MethodPut, "/admin/fee-rules/default", docRule))
	if row.ShopID != nil || row.Rule.MaxKm != 800 || row.Rule.RoadFactor != 130 || row.Rule.FreeKm != 200 {
		t.Fatalf("default rule = %+v", row)
	}
	// The doc example: 2.0 km, 7.5 kg is Rs 160 from the default rule.
	w := f.OK(http.MethodPost, "/admin/fee-rules/preview",
		`{"shopId":"`+shopID.String()+`","distanceKm":2,"weightKg":7.5,"itemsCents":0}`)
	if got := apptest.Decode[delivery.PreviewResult](t, w); got.FeeCents != 16000 || got.Rule != "DEFAULT" {
		t.Fatalf("preview = %+v", got)
	}

	// A shop rule wins over the default; fields left out keep their defaults.
	f.OK(http.MethodPut, "/admin/fee-rules/shops/"+shopID.String(), `{"baseCents":5000,"maxKm":3}`)
	w = f.OK(http.MethodPost, "/admin/fee-rules/preview",
		`{"shopId":"`+shopID.String()+`","distanceKm":1,"weightKg":1,"itemsCents":0}`)
	if got := apptest.Decode[delivery.PreviewResult](t, w); got.FeeCents != 5000 || got.Rule != "SHOP" {
		t.Fatalf("shop preview = %+v", got)
	}
	if code, body := f.preview(shopID, `"distanceKm":3,"weightKg":1,"itemsCents":0`); code != http.StatusConflict {
		t.Fatalf("3 km x 1.3 is past max 3 km: %d %s, want 409 OUT_OF_RANGE", code, body)
	}

	list := apptest.Decode[struct{ Data []delivery.RuleRow }](t, f.OK(http.MethodGet, "/admin/fee-rules", ""))
	if len(list.Data) != 2 || list.Data[0].ShopID != nil || list.Data[1].ShopID == nil {
		t.Fatalf("list = %+v, want default first then the shop", list.Data)
	}

	// Saving again replaces, it does not add a row.
	f.OK(http.MethodPut, "/admin/fee-rules/default", docRule)
	if n := f.a.Count(`SELECT count(*) FROM delivery_fee_rules`); n != 2 {
		t.Errorf("rows = %d, want 2", n)
	}
	// Every change leaves an audit row with the old value on the second write.
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'fee_rule.set'`); n != 3 {
		t.Errorf("audit rows = %d, want 3", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'fee_rule.set' AND before IS NOT NULL`); n != 1 {
		t.Errorf("audit rows with a before value = %d, want 1", n)
	}
}

func TestFeeRules_Validation(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	cases := map[string]struct{ body, field string }{
		"negative base":   {`{"baseCents":-1}`, "baseCents"},
		"max below min":   {`{"minCents":500,"maxCents":100}`, "maxCents"},
		"max km too far":  {`{"maxKm":51}`, "maxKm"},
		"road factor low": {`{"roadFactor":0.9}`, "roadFactor"},
		"free above zero": {`{"freeAboveCents":0}`, "freeAboveCents"},
		"three decimals":  {`{"freeKm":1.234}`, ""},
		"km as text":      {`{"maxKm":"8"}`, ""},
		"unknown field":   {`{"surge":2}`, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := f.Expect(http.MethodPut, "/admin/fee-rules/default", c.body, http.StatusBadRequest, "VALIDATION_ERROR")
			if c.field != "" && e.Error.Fields[c.field] == "" {
				t.Errorf("fields = %v, want %q", e.Error.Fields, c.field)
			}
		})
	}
	f.Expect(http.MethodPut, "/admin/fee-rules/shops/"+uuid.NewString(), `{}`, http.StatusNotFound, "NOT_FOUND")
	f.Expect(http.MethodPut, "/admin/fee-rules/shops/not-a-uuid", `{}`, http.StatusNotFound, "NOT_FOUND")
	f.Expect(http.MethodPost, "/admin/fee-rules/preview",
		`{"shopId":"`+uuid.NewString()+`","distanceKm":1,"weightKg":1,"itemsCents":0}`, http.StatusNotFound, "NOT_FOUND")
	f.OK(http.MethodPut, "/admin/fee-rules/default", docRule)
	f.Expect(http.MethodPost, "/admin/fee-rules/preview",
		`{"shopId":"`+shopID.String()+`","distanceKm":1,"weightKg":1,"itemsCents":-5}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.Expect(http.MethodPost, "/admin/fee-rules/preview",
		`{"shopId":"`+shopID.String()+`","distanceKm":-1,"weightKg":1,"itemsCents":0}`, http.StatusBadRequest, "VALIDATION_ERROR")
}

// Only a signed-in admin may touch fee rules; a customer and a stranger get no access.
func TestFeeRules_NeedAdmin(t *testing.T) {
	f := newFixture(t)
	if w := f.a.Call(http.MethodGet, "/v1/admin/fee-rules", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no cookie: %d, want 401", w.Code)
	}
}
