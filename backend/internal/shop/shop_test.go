package shop_test

import (
	"net/http"
	"strings"
	"testing"

	"gp/internal/shop"
	"gp/test/apptest"
)

func TestCreateShop(t *testing.T) {
	f := newFixture(t)
	w := f.call(http.MethodPost, "/admin/shops", newShop)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body)
	}
	d := apptest.Decode[shop.Detail](t, w)
	s := d.Shop
	if s.Status != "DRAFT" || s.AcceptingOrders || s.Slug != "krishna-super" || s.ThemeColor != "#222222" ||
		s.Phone == nil || *s.Phone != "+94771234567" {
		t.Fatalf("shop = %+v", s)
	}
	if d.Settings.PaymentProvider != "PAYHERE" || d.Settings.MarkupBP != 250 || len(d.Hours) != 0 || d.Readiness.Ready {
		t.Fatalf("defaults = %+v readiness %+v", d.Settings, d.Readiness)
	}

	// A site rebuild is queued and the change is audited.
	if n := f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'site.rebuild' AND shop_id = $1`, s.ID); n != 1 {
		t.Errorf("site.rebuild events = %d, want 1", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'shop.create' AND shop_id = $1 AND actor_role = 'SUPER_ADMIN'`, s.ID); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
}

func TestCreateShopRefusals(t *testing.T) {
	f := newFixture(t)
	f.createShop(newShop)
	with := func(old, new string) string { return strings.Replace(newShop, old, new, 1) }
	tests := map[string]struct {
		body  string
		field string
	}{
		"slug taken":       {newShop, "slug"},
		"slug upper case":  {with("krishna-super", "Krishna"), "slug"},
		"slug too short":   {with("krishna-super", "ab"), "slug"},
		"slug double dash": {with("krishna-super", "a--b"), "slug"},
		"name too short":   {with("Krishna Super", "K"), "name"},
		"lat outside":      {with("6.9271", "20.5"), "lat"},
		"lng outside":      {with("79.8612", "10"), "lng"},
		"bad phone":        {with("0771234567", "12345"), "phone"},
		"http logo":        {with(`"lat"`, `"logoUrl":"http://x.test/a.png","lat"`), "logoUrl"},
		"javascript logo":  {with(`"lat"`, `"logoUrl":"javascript:alert(1)","lat"`), "logoUrl"},
		"bad theme":        {with(`"lat"`, `"themeColor":"red","lat"`), "themeColor"},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			w := f.call(http.MethodPost, "/admin/shops", c.body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s", w.Code, w.Body)
			}
			if got := f.errorOf(w).Error.Fields; got[c.field] == "" {
				t.Fatalf("fields = %v, want %q", got, c.field)
			}
		})
	}

	// Mass assignment: status and accepting cannot be sent on create.
	for _, extra := range []string{`"status":"ACTIVE"`, `"acceptingOrders":true`, `"id":"00000000-0000-0000-0000-000000000001"`} {
		body := with(`"lat"`, extra+`,"lat"`)
		if w := f.call(http.MethodPost, "/admin/shops", strings.Replace(body, "krishna-super", "other-shop", 1)); w.Code != http.StatusBadRequest {
			t.Errorf("create with %s = %d, want 400", extra, w.Code)
		}
	}
	if n := f.a.Count(`SELECT count(*) FROM shops`); n != 1 {
		t.Fatalf("shops = %d, want 1", n)
	}
}

func TestUpdateShop(t *testing.T) {
	f := newFixture(t)
	d := f.createShop(newShop)
	id := d.Shop.ID
	rebuilds := func() int {
		return f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'site.rebuild' AND shop_id = $1`, id)
	}

	// A phone change is not baked into the site: no rebuild.
	d = apptest.Decode[shop.Detail](t, f.ok(http.MethodPatch, f.shopPath(id, ""), `{"phone":"0712345678","addressText":"New road"}`))
	if *d.Shop.Phone != "+94712345678" || *d.Shop.AddressText != "New road" || rebuilds() != 1 {
		t.Fatalf("phone update: %+v rebuilds %d", d.Shop, rebuilds())
	}
	// A name, logo, slug or status change is.
	f.ok(http.MethodPatch, f.shopPath(id, ""), `{"name":"Krishna Supermarket"}`)
	f.ok(http.MethodPatch, f.shopPath(id, ""), `{"logoUrl":"https://cdn.example.test/logo.png"}`)
	f.ok(http.MethodPatch, f.shopPath(id, ""), `{"slug":"krishna"}`)
	f.ok(http.MethodPatch, f.shopPath(id, ""), `{"status":"ACTIVE"}`)
	if rebuilds() != 5 {
		t.Errorf("rebuild events = %d, want 5", rebuilds())
	}
	// An empty string clears an optional field.
	d = apptest.Decode[shop.Detail](t, f.ok(http.MethodPatch, f.shopPath(id, ""), `{"logoUrl":""}`))
	if d.Shop.LogoURL != nil {
		t.Errorf("logo not cleared: %v", *d.Shop.LogoURL)
	}

	for name, body := range map[string]string{
		"bad status":    `{"status":"OPEN"}`,
		"bad slug":      `{"slug":"No Spaces"}`,
		"unknown field": `{"acceptingOrders":true}`,
		"id in body":    `{"id":"00000000-0000-0000-0000-000000000001"}`,
	} {
		if w := f.call(http.MethodPatch, f.shopPath(id, ""), body); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, w.Code)
		}
	}
	if w := f.call(http.MethodPatch, f.shopPath(id, ""), `{"slug":"krishna"}`); w.Code != http.StatusOK {
		t.Errorf("keeping the same slug = %d", w.Code)
	}
	other := f.createShop(strings.Replace(newShop, "krishna-super", "other-shop", 1))
	if w := f.call(http.MethodPatch, f.shopPath(other.Shop.ID, ""), `{"slug":"krishna"}`); w.Code != http.StatusBadRequest {
		t.Errorf("taking another shop's slug = %d, want 400", w.Code)
	}
	if w := f.call(http.MethodGet, f.shopPath(randomID(), ""), ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown shop = %d, want 404", w.Code)
	}
}

func TestSuspendingStopsOrders(t *testing.T) {
	f := newFixture(t)
	id := f.makeReady("krishna-super")
	f.ok(http.MethodPost, f.shopPath(id, "/accepting"), `{"on":true}`)

	d := apptest.Decode[shop.Detail](t, f.ok(http.MethodPatch, f.shopPath(id, ""), `{"status":"SUSPENDED"}`))
	if d.Shop.AcceptingOrders {
		t.Fatal("a suspended shop must stop accepting orders")
	}
}

func TestPublicList(t *testing.T) {
	f := newFixture(t)
	live := f.createShop(newShop)
	f.createShop(strings.Replace(newShop, "krishna-super", "draft-shop", 1))
	f.ok(http.MethodPatch, f.shopPath(live.Shop.ID, ""), `{"status":"ACTIVE","logoUrl":"https://cdn.example.test/k.png"}`)

	w := f.a.Call(http.MethodGet, "/v1/shops?fields=slug", "") // no sign-in
	if w.Code != http.StatusOK {
		t.Fatalf("public list = %d %s", w.Code, w.Body)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "public") {
		t.Errorf("Cache-Control = %q, want public", cc)
	}
	got := apptest.Decode[struct{ Data []shop.PublicShop }](t, w).Data
	if len(got) != 1 || got[0].Slug != "krishna-super" || got[0].LogoURL == nil {
		t.Fatalf("list = %+v", got)
	}
	// Nothing private leaks through the public shape.
	for _, banned := range []string{"phone", "lat", "status", "settings", "acceptingOrders"} {
		if strings.Contains(w.Body.String(), `"`+banned+`"`) {
			t.Errorf("public list contains %q", banned)
		}
	}
	for _, q := range []string{"", "?fields=all", "?fields=slug,name"} {
		if w := f.a.Call(http.MethodGet, "/v1/shops"+q, ""); w.Code != http.StatusBadRequest {
			t.Errorf("GET /shops%s = %d, want 400", q, w.Code)
		}
	}
}
