package coupon_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/coupon"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

type list struct {
	Data []coupon.Coupon `json:"data"`
}

func window() (string, string) {
	now := time.Now().UTC()
	return now.Add(-time.Hour).Format(time.RFC3339), now.Add(24 * time.Hour).Format(time.RFC3339)
}

// body builds a coupon body; extra fields are added or replaced by the pairs.
func body(pairs ...any) string {
	starts, ends := window()
	m := map[string]any{"code": "WELCOME10", "type": "PERCENT", "value": 1000, "startsAt": starts, "endsAt": ends, "fundedBy": "PLATFORM"}
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i+1] == nil {
			delete(m, pairs[i].(string))
		} else {
			m[pairs[i].(string)] = pairs[i+1]
		}
	}
	raw, _ := json.Marshal(m)
	return string(raw)
}

func create(t *testing.T, admin apptest.Client, b string) coupon.Coupon {
	t.Helper()
	w := admin.Call(http.MethodPost, "/admin/coupons", b)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body)
	}
	return apptest.Decode[coupon.Coupon](t, w)
}

// refused fails the test unless the call is a validation error.
func refused(t *testing.T, admin apptest.Client, name, method, path, b string) {
	t.Helper()
	if w := admin.Call(method, path, b); w.Code != http.StatusBadRequest || apptest.ErrorOf(t, w).Error.Code != "VALIDATION_ERROR" {
		t.Errorf("%s = %d %s, want 400 VALIDATION_ERROR", name, w.Code, w.Body)
	}
}

func TestAdmin_CreateListUpdate(t *testing.T) {
	a := apptest.New(t)
	admin := a.AsAdmin()

	c := create(t, admin, body("code", " welcome10 ", "maxDiscountCents", 50000, "minOrderCents", 100000, "usageLimit", 100, "perUserLimit", 2))
	if c.Code != "WELCOME10" || c.Type != enum.CouponTypePercent || c.Value != 1000 || c.Status != enum.CouponStatusActive || c.UsedCount != 0 ||
		c.PerUserLimit != 2 || *c.UsageLimit != 100 || *c.MaxDiscountCents != 50000 || c.MinOrderCents != 100000 || c.ShopID != nil || c.AllowLoss {
		t.Fatalf("created = %+v", c)
	}
	if again := create2(t, admin, body("code", "welcome10")); again != "taken" {
		t.Errorf("same code in lower case: code field = %q, want taken", again)
	}
	if d := create(t, admin, body("code", "TEA5", "type", "FIXED", "value", 5000, "fundedBy", "PLATFORM", "allowLoss", true)); d.PerUserLimit != 1 || !d.AllowLoss || d.UsageLimit != nil {
		t.Errorf("fixed coupon = %+v, want one use per customer, no total limit, allow loss", d)
	}

	// A shop-funded coupon belongs to a shop.
	shop := a.SeedShop("krishna")
	for name, b := range map[string]string{
		"shop-funded for all shops": body("code", "SHOPPAYS", "fundedBy", "SHOP"),
		"unknown shop":              body("code", "NOSHOP", "shopId", uuid.NewString()),
	} {
		refused(t, admin, name, http.MethodPost, "/admin/coupons", b)
	}
	s := create(t, admin, body("code", "SHOPPAYS", "fundedBy", "SHOP", "shopId", shop.String()))
	if s.ShopID == nil || *s.ShopID != shop || s.FundedBy != enum.FundedByShop {
		t.Errorf("shop coupon = %+v", s)
	}

	all := apptest.Decode[list](t, admin.OK(http.MethodGet, "/admin/coupons", "")).Data
	if len(all) != 3 || all[0].Code != "SHOPPAYS" {
		t.Fatalf("list = %d coupons, first %q; want 3, newest first", len(all), all[0].Code)
	}
	if got := apptest.Decode[list](t, admin.OK(http.MethodGet, "/admin/coupons?shopId="+shop.String(), "")).Data; len(got) != 1 {
		t.Errorf("by shop = %d", len(got))
	}
	for _, q := range []string{"?status=NOPE", "?shopId=x"} {
		admin.Expect(http.MethodGet, "/admin/coupons"+q, "", http.StatusBadRequest, "VALIDATION_ERROR")
	}

	// Pause, change the limits, remove a limit with null.
	path := "/admin/coupons/" + c.ID.String()
	p := apptest.Decode[coupon.Coupon](t, admin.OK(http.MethodPatch, path, `{"status":"PAUSED","usageLimit":null,"maxDiscountCents":null,"perUserLimit":3,"minOrderCents":0}`))
	if p.Status != enum.CouponStatusPaused || p.UsageLimit != nil || p.MaxDiscountCents != nil || p.PerUserLimit != 3 || p.MinOrderCents != 0 || p.Code != c.Code || p.Value != c.Value {
		t.Errorf("patched = %+v", p)
	}
	if got := apptest.Decode[list](t, admin.OK(http.MethodGet, "/admin/coupons?status=PAUSED", "")).Data; len(got) != 1 || got[0].ID != c.ID {
		t.Errorf("paused list = %+v", got)
	}
	if a.Count(`SELECT count(*) FROM audit_log WHERE action IN ('coupon.create', 'coupon.update')`) != 4 {
		t.Errorf("audit rows = %d, want 3 creates and 1 update", a.Count(`SELECT count(*) FROM audit_log WHERE action IN ('coupon.create', 'coupon.update')`))
	}
	if a.Count(`SELECT count(*) FROM audit_log WHERE action = 'coupon.update' AND before->>'status' = 'ACTIVE' AND after->>'status' = 'PAUSED'`) != 1 {
		t.Error("the update audit row has no before and after")
	}
}

func create2(t *testing.T, admin apptest.Client, b string) string {
	t.Helper()
	w := admin.Call(http.MethodPost, "/admin/coupons", b)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create = %d %s, want 400", w.Code, w.Body)
	}
	return apptest.ErrorOf(t, w).Error.Fields["code"]
}

func TestAdmin_CreateRefusals(t *testing.T) {
	a := apptest.New(t)
	admin := a.AsAdmin()
	starts, ends := window()
	cases := map[string]struct{ body, field string }{
		"short code":        {body("code", "AB"), "code"},
		"symbol in code":    {body("code", "SAVE-10"), "code"},
		"long code":         {body("code", strings.Repeat("A", 21)), "code"},
		"bad type":          {body("type", "BOGUS"), "type"},
		"percent over 100":  {body("value", 10001), "value"},
		"percent zero":      {body("value", 0), "value"},
		"fixed negative":    {body("type", "FIXED", "value", -5), "value"},
		"bad funder":        {body("fundedBy", "THE_KING"), "fundedBy"},
		"no start":          {body("startsAt", nil), "startsAt"},
		"ends before start": {body("startsAt", ends, "endsAt", starts), "endsAt"},
		"already ended":     {body("startsAt", "2020-01-01T00:00:00Z", "endsAt", "2020-02-01T00:00:00Z"), "endsAt"},
		"max discount zero": {body("maxDiscountCents", 0), "maxDiscountCents"},
		"limit zero":        {body("usageLimit", 0), "usageLimit"},
		"per user zero":     {body("perUserLimit", -1), "perUserLimit"},
		"min order below 0": {body("minOrderCents", -1), "minOrderCents"},
		"allow loss, shop":  {body("fundedBy", "SHOP", "shopId", uuid.NewString(), "allowLoss", true), "allowLoss"},
	}
	for name, c := range cases {
		w := admin.Call(http.MethodPost, "/admin/coupons", c.body)
		e := apptest.ErrorOf(t, w)
		if w.Code != http.StatusBadRequest || e.Error.Code != "VALIDATION_ERROR" || e.Error.Fields[c.field] == "" {
			t.Errorf("%s = %d %s, want 400 with %s", name, w.Code, w.Body, c.field)
		}
	}
	admin.Expect(http.MethodPost, "/admin/coupons", body("discount", 5), http.StatusBadRequest, "VALIDATION_ERROR")
	admin.Expect(http.MethodPost, "/admin/coupons", `not json`, http.StatusBadRequest, "VALIDATION_ERROR")
	if a.Count(`SELECT count(*) FROM coupons`) != 0 {
		t.Error("a refused coupon was saved")
	}
}

func TestAdmin_UpdateRefusals(t *testing.T) {
	a := apptest.New(t)
	admin := a.AsAdmin()
	c := create(t, admin, body("usageLimit", 5))
	shopFunded := create(t, admin, body("code", "SHOPPAYS", "fundedBy", "SHOP", "shopId", a.SeedShop("krishna").String()))
	a.Exec(`UPDATE coupons SET used_count = 3 WHERE id = $1`, c.ID)
	path := "/admin/coupons/" + c.ID.String()

	for name, b := range map[string]string{
		"empty body":              `{}`,
		"limit below the used":    `{"usageLimit":2}`,
		"bad status":              `{"status":"DELETED"}`,
		"null status":             `{"status":null}`,
		"end before start":        `{"endsAt":"2000-01-01T00:00:00Z"}`,
		"per user zero":           `{"perUserLimit":0}`,
		"null per user":           `{"perUserLimit":null}`,
		"the value cannot change": `{"value":5}`,
		"the code cannot change":  `{"code":"OTHER"}`,
		"the funder cannot":       `{"fundedBy":"SHOP"}`,
	} {
		refused(t, admin, name, http.MethodPatch, path, b)
	}
	admin.Expect(http.MethodPatch, "/admin/coupons/"+shopFunded.ID.String(), `{"allowLoss":true}`, http.StatusBadRequest, "VALIDATION_ERROR")
	admin.Expect(http.MethodPatch, "/admin/coupons/"+uuid.NewString(), `{"status":"PAUSED"}`, http.StatusNotFound, "NOT_FOUND")
	admin.Expect(http.MethodPatch, "/admin/coupons/not-an-id", `{"status":"PAUSED"}`, http.StatusNotFound, "NOT_FOUND")
	if got := apptest.Decode[coupon.Coupon](t, admin.OK(http.MethodPatch, path, `{"usageLimit":3}`)); *got.UsageLimit != 3 || got.UsedCount != 3 {
		t.Errorf("limit equal to the used = %+v, want allowed", got)
	}
	if a.Count(`SELECT count(*) FROM coupons WHERE id = $1 AND status = 'ACTIVE'`, c.ID) != 1 {
		t.Error("a refused patch changed the coupon")
	}
}

// Coupons are an admin's tool: nobody else lists, makes or changes them.
func TestAdmin_OnlyTheAdmin(t *testing.T) {
	a := apptest.New(t)
	admin := a.AsAdmin()
	c := create(t, admin, body())
	user := a.As(a.SignedIn(fakes.Claims{Sub: "nimal", Email: "nimal@example.com", Name: "Nimal"}))
	shop := a.SeedShop("krishna")
	owner := a.As(a.SignedIn(fakes.Claims{Sub: "owner", Email: "owner@example.com", Name: "Owner"}))
	a.Grant(owner.Session.Me.ID, &shop, "SHOP_OWNER")

	for who, client := range map[string]apptest.Client{"customer": user, "shop owner": owner} {
		for _, call := range [][3]string{{http.MethodGet, "/admin/coupons", ""}, {http.MethodPost, "/admin/coupons", body("code", "SNEAKY")}, {http.MethodPatch, "/admin/coupons/" + c.ID.String(), `{"status":"PAUSED"}`}} {
			if w := client.Call(call[0], call[1], call[2]); w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
				t.Errorf("%s %s %s = %d, want 403 or 404", who, call[0], call[1], w.Code)
			}
		}
	}
	if w := a.Call(http.MethodGet, "/v1/admin/coupons", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no login = %d, want 401", w.Code)
	}
	if a.Count(`SELECT count(*) FROM coupons`) != 1 {
		t.Error("somebody who is not the admin changed the coupons")
	}
}
