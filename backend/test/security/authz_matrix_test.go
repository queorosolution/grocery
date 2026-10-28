// Package security holds the tests from docs/11 that walk the whole API.
package security_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	apihttp "gp/internal/http"
	"gp/internal/shared/authz"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

// The matrix is built from two generated sources: the real route table and the
// role / permission map from contracts/enums.json. A new route or permission is
// covered the moment it is registered; nothing here lists routes by hand.
//
// Only the guard is judged. A role that is allowed must get past it (anything
// but 401 or 403); a role that is not allowed must be stopped with exactly the
// code the guard owes: 403 for a member without the permission, 404 for
// someone with no membership in the shop.

type actors struct {
	customer, outsider            apptest.Session
	owner, staff, staffCanEdit    apptest.Session
	driver, platformDriver, admin apptest.Session
	adminNoTotp                   apptest.Session
}

type world struct {
	a     *apptest.App
	shopA uuid.UUID
	who   actors
}

func newWorld(t *testing.T) world {
	t.Helper()
	a := apptest.New(t)
	w := world{a: a, shopA: a.SeedShop("alpha")}
	shopB := a.SeedShop("beta")
	in := func(sub string) apptest.Session {
		return a.SignedIn(fakes.Claims{Sub: sub, Email: sub + "@example.test"})
	}
	w.who.customer = in("customer")
	w.who.owner, w.who.staff, w.who.staffCanEdit = in("owner"), in("staff"), in("staff-edit")
	w.who.driver, w.who.platformDriver, w.who.outsider = in("driver"), in("platform-driver"), in("outsider")
	a.Grant(w.who.owner.Me.ID, &w.shopA, string(enum.RoleShopOwner))
	a.Grant(w.who.staff.Me.ID, &w.shopA, string(enum.RoleShopStaff))
	a.Grant(w.who.staffCanEdit.Me.ID, &w.shopA, string(enum.RoleShopStaff))
	a.Exec(`UPDATE memberships SET can_edit_products = true WHERE user_id = $1`, w.who.staffCanEdit.Me.ID)
	a.Grant(w.who.driver.Me.ID, &w.shopA, string(enum.RoleShopDriver))
	a.Grant(w.who.platformDriver.Me.ID, nil, string(enum.RolePlatformDriver))
	a.Grant(w.who.outsider.Me.ID, &shopB, string(enum.RoleShopOwner))
	// Signed in before the admin passes TOTP, so this session never does.
	w.who.adminNoTotp = a.SignedIn(fakes.Claims{Sub: "boss", Email: apptest.AdminEmail})
	w.who.admin = a.Admin()
	return w
}

// call sends the route with a body that is valid JSON but otherwise empty and
// every path value filled in. The guard runs before the body is read.
func (w world) call(rt apihttp.Route, cookies []*http.Cookie) *httptest.ResponseRecorder {
	path := "/v1" + rt.Pattern
	path = strings.ReplaceAll(path, "{shopId}", w.shopA.String())
	for strings.Contains(path, "{") {
		i, j := strings.Index(path, "{"), strings.Index(path, "}")
		path = path[:i] + uuid.NewString() + path[j+1:]
	}
	body := ""
	if rt.Method != http.MethodGet && rt.Method != http.MethodDelete {
		body = "{}"
	}
	return w.a.Call(rt.Method, path, body, cookies...)
}

func passed(code int) bool { return code != http.StatusUnauthorized && code != http.StatusForbidden }

func TestAuthzMatrix(t *testing.T) {
	w := newWorld(t)
	routes := apihttp.Routes(w.a.Deps)
	var seenPerm, seenAdmin int

	for _, rt := range routes {
		rt := rt
		name := rt.Method + " " + rt.Pattern
		switch {
		case rt.Guard == apihttp.Admin:
			seenAdmin++
			t.Run(name, func(t *testing.T) { checkAdmin(t, w, rt) })
		case strings.HasPrefix(string(rt.Guard), "perm:"):
			seenPerm++
			perm := enum.Permission(strings.TrimPrefix(string(rt.Guard), "perm:"))
			t.Run(name, func(t *testing.T) { checkPerm(t, w, rt, perm) })
		}
	}
	if seenPerm == 0 || seenAdmin == 0 {
		t.Fatalf("the route table has %d perm routes and %d admin routes; the matrix is empty", seenPerm, seenAdmin)
	}
}

// AUTHZ-07: every admin route stops everyone who is not a TOTP-verified super admin.
func checkAdmin(t *testing.T, w world, rt apihttp.Route) {
	want := func(who string, got *httptest.ResponseRecorder, code int) {
		t.Helper()
		if got.Code != code {
			t.Errorf("%s: got %d, want %d (%s)", who, got.Code, code, got.Body)
		}
	}
	want("anonymous", w.call(rt, nil), http.StatusUnauthorized)
	for who, s := range map[string]apptest.Session{
		"customer": w.who.customer, "owner": w.who.owner, "staff": w.who.staff,
		"driver": w.who.driver, "platform driver": w.who.platformDriver,
	} {
		want(who, w.call(rt, s.Cookies()), http.StatusForbidden)
	}
	want("admin without TOTP", w.call(rt, w.who.adminNoTotp.Cookies()), http.StatusUnauthorized)
	if got := w.call(rt, w.who.admin.Cookies()); !passed(got.Code) {
		t.Errorf("admin with TOTP: got %d (%s)", got.Code, got.Body)
	}
}

// AUTHZ-02 and the permission map: every role is checked against the generated
// role_permissions for this route's permission.
func checkPerm(t *testing.T, w world, rt apihttp.Route, perm enum.Permission) {
	shopRoute := strings.Contains(rt.Pattern, "{shopId}")

	if got := w.call(rt, nil); got.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: got %d, want 401", got.Code)
	}
	type member struct {
		who     string
		session apptest.Session
		role    enum.Role
		canEdit bool
	}
	members := []member{
		{"owner", w.who.owner, enum.RoleShopOwner, false},
		{"staff", w.who.staff, enum.RoleShopStaff, false},
		{"staff with product edit", w.who.staffCanEdit, enum.RoleShopStaff, true},
		{"shop driver", w.who.driver, enum.RoleShopDriver, false},
		{"platform driver", w.who.platformDriver, enum.RolePlatformDriver, false},
	}
	for _, m := range members {
		allowed := authz.Membership{Role: m.role, CanEditProducts: m.canEdit}.Has(perm)
		got := w.call(rt, m.session.Cookies())
		switch {
		case m.role == enum.RolePlatformDriver && shopRoute:
			if got.Code != http.StatusNotFound {
				t.Errorf("%s on a shop route: got %d, want 404", m.who, got.Code)
			}
		case allowed:
			if !passed(got.Code) {
				t.Errorf("%s: got %d, want the guard to let them in (%s)", m.who, got.Code, got.Body)
			}
		default:
			if got.Code != http.StatusForbidden {
				t.Errorf("%s: got %d, want 403", m.who, got.Code)
			}
		}
	}

	// People with no role in this shop: 404 on shop routes (nothing to learn
	// about the shop). On routes without a shop the answer follows their own
	// roles: a customer holds nothing, and an owner holds what owners hold.
	ownerHas := authz.Membership{Role: enum.RoleShopOwner}.Has(perm)
	for who, s := range map[string]apptest.Session{"customer": w.who.customer, "owner of another shop": w.who.outsider} {
		got := w.call(rt, s.Cookies())
		switch {
		case shopRoute:
			if got.Code != http.StatusNotFound {
				t.Errorf("%s: got %d, want 404", who, got.Code)
			}
		case who == "customer" || !ownerHas:
			if got.Code != http.StatusForbidden {
				t.Errorf("%s: got %d, want 403", who, got.Code)
			}
		}
	}

	// The super admin needs the TOTP check wherever the role holds the permission.
	if authz.RoleHas(enum.RoleSuperAdmin, perm) {
		if got := w.call(rt, w.who.adminNoTotp.Cookies()); got.Code != http.StatusUnauthorized {
			t.Errorf("admin without TOTP: got %d, want 401", got.Code)
		}
		if got := w.call(rt, w.who.admin.Cookies()); !passed(got.Code) {
			t.Errorf("admin with TOTP: got %d (%s)", got.Code, got.Body)
		}
	} else if shopRoute {
		if got := w.call(rt, w.who.admin.Cookies()); got.Code != http.StatusNotFound {
			t.Errorf("admin without the permission: got %d, want 404", got.Code)
		}
	}
}

// Every route is either public, signed, user, admin or a permission route, and
// admin routes live under /admin (the route table test in internal/http covers
// the guard names; this one covers the layout).
func TestRouteLayout(t *testing.T) {
	w := newWorld(t)
	for _, rt := range apihttp.Routes(w.a.Deps) {
		name := rt.Method + " " + rt.Pattern
		if strings.HasPrefix(rt.Pattern, "/admin/") && rt.Guard != apihttp.Admin {
			t.Errorf("%s is under /admin but guarded %q", name, rt.Guard)
		}
		if rt.Guard == apihttp.Admin && !strings.HasPrefix(rt.Pattern, "/admin/") {
			t.Errorf("%s has the admin guard outside /admin", name)
		}
		if strings.HasPrefix(rt.Pattern, "/console/shops/{shopId}/") && !strings.HasPrefix(string(rt.Guard), "perm:") {
			t.Errorf("%s is a shop console route without a permission guard (%q)", name, rt.Guard)
		}
	}
}
