package shop_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"gp/internal/shop"
	"gp/test/apptest"
)

const secretValue = "S3cr3t-Merchant-Value-9d41"

func (f fixture) payhere(body string) *shop.Account {
	f.a.T.Helper()
	a := apptest.Decode[shop.Account](f.a.T, f.ok(http.MethodPost, "/admin/payment-accounts", body))
	return &a
}

const payhereBody = `{"ownerType":"PLATFORM","provider":"PAYHERE","mode":"SANDBOX","merchantId":"1230001","merchantSecret":"` + secretValue + `"}`

func TestCreatePayHereAccount(t *testing.T) {
	f := newFixture(t)
	w := f.call(http.MethodPost, "/admin/payment-accounts", payhereBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body)
	}
	acc := apptest.Decode[shop.Account](t, w)
	if acc.Provider != "PAYHERE" || acc.Status != "ACTIVE" || !acc.HasSecret || acc.MerchantID == nil || *acc.MerchantID != "1230001" || acc.Mode != "SANDBOX" {
		t.Fatalf("account = %+v", acc)
	}
	// The secret went to the secret store under payhere/<id>, and nowhere else.
	stored, _ := f.a.Secrets.Get(context.Background(), "payhere/"+acc.ID.String())
	if stored != secretValue {
		t.Fatalf("stored secret = %q", stored)
	}
}

// DATA-08: no response, audit row or table column outside the secret store
// holds the merchant secret.
func TestMerchantSecretNeverLeaves(t *testing.T) {
	f := newFixture(t)
	acc := f.payhere(payhereBody)
	rotated := "Rotated-Value-77aa-ZZ"
	responses := []string{
		f.ok(http.MethodGet, "/admin/payment-accounts", "").Body.String(),
		f.ok(http.MethodPatch, "/admin/payment-accounts/"+acc.ID.String(), `{"merchantSecret":"`+rotated+`"}`).Body.String(),
		f.ok(http.MethodGet, "/admin/payment-accounts", "").Body.String(),
		f.ok(http.MethodGet, "/admin/audit", "").Body.String(),
		f.ok(http.MethodGet, "/admin/audit?entity=payment_account", "").Body.String(),
		f.ok(http.MethodGet, "/admin/shops", "").Body.String(),
	}
	for i, body := range responses {
		for _, secret := range []string{secretValue, rotated} {
			if strings.Contains(body, secret) {
				t.Errorf("response %d contains a merchant secret", i)
			}
		}
		if strings.Contains(body, "secretRef") || strings.Contains(body, "secret_ref") || strings.Contains(body, "payhere/") {
			t.Errorf("response %d exposes the secret reference", i)
		}
	}
	for _, table := range []string{"audit_log", "payment_accounts", "outbox"} {
		var hits int
		q := `SELECT count(*) FROM ` + table + ` t WHERE t::text LIKE '%' || $1 || '%' OR t::text LIKE '%' || $2 || '%'`
		if err := f.a.Env.Owner.QueryRow(context.Background(), q, secretValue, rotated).Scan(&hits); err != nil || hits != 0 {
			t.Errorf("%s holds a secret value (%d rows, err %v)", table, hits, err)
		}
	}
	// The rotation replaced the stored value and says so in the audit trail.
	stored, _ := f.a.Secrets.Get(context.Background(), "payhere/"+acc.ID.String())
	if stored != rotated {
		t.Errorf("stored secret = %q after rotation", stored)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'payment_account.update' AND after->>'secretRotated' = 'true'`); n != 1 {
		t.Errorf("rotation audit rows = %d, want 1", n)
	}
}

func TestPaymentAccountRules(t *testing.T) {
	f := newFixture(t)
	shopID := f.createShop(newShop).Shop.ID.String()
	tests := map[string]struct {
		body  string
		field string
	}{
		"payhere without secret":   {`{"ownerType":"PLATFORM","provider":"PAYHERE","merchantId":"123"}`, "merchantSecret"},
		"payhere without merchant": {`{"ownerType":"PLATFORM","provider":"PAYHERE","merchantSecret":"` + secretValue + `"}`, "merchantId"},
		"short secret":             {`{"ownerType":"PLATFORM","provider":"PAYHERE","merchantId":"123","merchantSecret":"abc"}`, "merchantSecret"},
		"secret with a space":      {`{"ownerType":"PLATFORM","provider":"PAYHERE","merchantId":"123","merchantSecret":"has a space inside"}`, "merchantSecret"},
		"bad merchant id":          {`{"ownerType":"PLATFORM","provider":"PAYHERE","merchantId":"12 3!","merchantSecret":"` + secretValue + `"}`, "merchantId"},
		"manual with credentials":  {`{"ownerType":"PLATFORM","provider":"MANUAL","merchantId":"123"}`, "merchantId"},
		"platform with a shop":     {`{"ownerType":"PLATFORM","shopId":"` + shopID + `","provider":"MANUAL"}`, "shopId"},
		"shop account without id":  {`{"ownerType":"SHOP","provider":"MANUAL"}`, "shopId"},
		"unknown shop":             {`{"ownerType":"SHOP","shopId":"00000000-0000-0000-0000-000000000001","provider":"MANUAL"}`, "shopId"},
		"bad provider":             {`{"ownerType":"PLATFORM","provider":"STRIPE"}`, "provider"},
		"bad mode":                 {`{"ownerType":"PLATFORM","provider":"MANUAL","mode":"TEST"}`, "mode"},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			w := f.call(http.MethodPost, "/admin/payment-accounts", c.body)
			if w.Code != http.StatusBadRequest || f.errorOf(w).Error.Fields[c.field] == "" {
				t.Fatalf("got %d %s, want 400 with %q", w.Code, w.Body, c.field)
			}
			if strings.Contains(w.Body.String(), secretValue) {
				t.Fatal("the error echoes the secret")
			}
		})
	}
	if w := f.call(http.MethodPost, "/admin/payment-accounts", `{"ownerType":"PLATFORM","provider":"MANUAL","status":"ACTIVE"}`); w.Code != http.StatusBadRequest {
		t.Errorf("status on create = %d, want 400", w.Code)
	}
	if n := f.a.Count(`SELECT count(*) FROM payment_accounts`); n != 0 {
		t.Fatalf("refused requests created %d accounts", n)
	}
}

func TestUpdatePaymentAccount(t *testing.T) {
	f := newFixture(t)
	acc := f.payhere(payhereBody)
	path := "/admin/payment-accounts/" + acc.ID.String()

	got := apptest.Decode[shop.Account](t, f.ok(http.MethodPatch, path, `{"mode":"LIVE","merchantId":"7770001","status":"DISABLED"}`))
	if got.Mode != "LIVE" || *got.MerchantID != "7770001" || got.Status != "DISABLED" || !got.HasSecret {
		t.Fatalf("updated = %+v", got)
	}
	// Re-enabling works because the credentials are still there.
	if got := apptest.Decode[shop.Account](t, f.ok(http.MethodPatch, path, `{"status":"ACTIVE"}`)); got.Status != "ACTIVE" {
		t.Fatalf("re-enable = %+v", got)
	}
	for name, body := range map[string]string{
		"not connected": `{"status":"NOT_CONNECTED"}`,
		"bad mode":      `{"mode":"x"}`,
		"short secret":  `{"merchantSecret":"x"}`,
		"provider swap": `{"provider":"MANUAL"}`,
		"owner swap":    `{"ownerType":"SHOP"}`,
		"shop move":     `{"shopId":"00000000-0000-0000-0000-000000000001"}`,
	} {
		if w := f.call(http.MethodPatch, path, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, w.Code)
		}
	}

	manual := f.payhere(`{"ownerType":"PLATFORM","provider":"MANUAL"}`)
	if w := f.call(http.MethodPatch, "/admin/payment-accounts/"+manual.ID.String(), `{"merchantSecret":"`+secretValue+`"}`); w.Code != http.StatusBadRequest {
		t.Errorf("secret on a manual account = %d, want 400", w.Code)
	}
	if w := f.call(http.MethodPatch, "/admin/payment-accounts/"+manual.ID.String()[:35]+"0", `{"mode":"LIVE"}`); w.Code != http.StatusNotFound {
		t.Errorf("unknown account = %d, want 404", w.Code)
	}
}

// A disabled account is no longer a usable payment method for the shop.
func TestDisablingAccountBreaksReadiness(t *testing.T) {
	f := newFixture(t)
	id := f.makeReady("krishna-super")
	var accID string
	if err := f.a.Env.Owner.QueryRow(context.Background(), `SELECT payment_account_id::text FROM shop_settings WHERE shop_id = $1`, id).Scan(&accID); err != nil {
		t.Fatal(err)
	}
	if r := apptest.Decode[shop.Readiness](t, f.ok(http.MethodGet, f.shopPath(id, "/readiness"), "")); !r.Ready {
		t.Fatalf("not ready to begin with: %+v", r)
	}
	f.ok(http.MethodPatch, "/admin/payment-accounts/"+accID, `{"status":"DISABLED"}`)
	r := apptest.Decode[shop.Readiness](t, f.ok(http.MethodGet, f.shopPath(id, "/readiness"), ""))
	if r.Ready || len(r.Missing) != 1 || r.Missing[0] != shop.MissingPayment {
		t.Fatalf("readiness = %+v", r)
	}
}

func TestAuditFeed(t *testing.T) {
	f := newFixture(t)
	d := f.createShop(newShop)
	f.ok(http.MethodPatch, f.shopPath(d.Shop.ID, ""), `{"name":"Renamed Shop"}`)

	type feed struct {
		Data []struct {
			Action string
			Before map[string]any
			After  map[string]any
		}
		NextCursor string
	}
	got := apptest.Decode[feed](t, f.ok(http.MethodGet, "/admin/audit?entity=shop&entityId="+d.Shop.ID.String(), ""))
	if len(got.Data) != 2 || got.Data[0].Action != "shop.update" || got.Data[1].Action != "shop.create" {
		t.Fatalf("feed = %+v", got)
	}
	if got.Data[0].Before["name"] != "Krishna Super" || got.Data[0].After["name"] != "Renamed Shop" {
		t.Fatalf("update row = %+v", got.Data[0])
	}
	for _, q := range []string{"?cursor=abc", "?cursor=-3", "?entity=" + strings.Repeat("x", 41)} {
		if w := f.call(http.MethodGet, "/admin/audit"+q, ""); w.Code != http.StatusBadRequest {
			t.Errorf("GET /admin/audit%s = %d, want 400", q[:min(len(q), 12)], w.Code)
		}
	}
}
