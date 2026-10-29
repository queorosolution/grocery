package security_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	apihttp "gp/internal/http"
	"gp/internal/shared/authz"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

// This file is the other half of the matrix: the routes that are not behind a
// permission or the admin guard, and the checks that need real data of two
// people. Like the first half it starts from the route table.

// roleChecked are user routes that let any signed-in user through the guard and
// then ask for a role of their own; a plain customer is refused with 403.
var roleChecked = map[string]string{
	"POST /auth/totp/setup":  "only a super admin may set up the authenticator",
	"POST /auth/totp/verify": "only a super admin may verify the authenticator",
	"POST /uploads/presign":  "the admin, or a member with product.edit for the shop in the body",
}

func (w world) callWith(rt apihttp.Route, body string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	path := "/v1" + rt.Pattern
	path = strings.ReplaceAll(path, "{shopId}", w.shopA.String())
	for strings.Contains(path, "{") {
		i, j := strings.Index(path, "{"), strings.Index(path, "}")
		path = path[:i] + uuid.NewString() + path[j+1:]
	}
	return w.a.CallWith(apptest.DefaultIP, rt.Method, path, body, map[string]string{"Idempotency-Key": "security-test-key-0001"}, cookies...)
}

// Public routes let everyone in; user routes need a session; signed routes
// ignore a session and need their signature. Nothing answers with a server error.
func TestAuthzMatrix_PublicUserAndSignedRoutes(t *testing.T) {
	w := newWorld(t)
	var public, user, signed int
	for _, rt := range apihttp.Routes(w.a.Deps) {
		name := rt.Method + " " + rt.Pattern
		body := ""
		if rt.Method != http.MethodGet {
			body = "{}"
		}
		switch {
		case rt.Guard == apihttp.Public:
			public++
			t.Run(name, func(t *testing.T) {
				got := w.callWith(rt, body, nil)
				if got.Code == http.StatusForbidden || got.Code >= 500 || (rt.Method == http.MethodGet && got.Code == http.StatusUnauthorized) {
					t.Errorf("anonymous: got %d (%s)", got.Code, got.Body)
				}
			})
		case rt.Guard == apihttp.User:
			user++
			t.Run(name, func(t *testing.T) {
				if got := w.callWith(rt, body, nil); got.Code != http.StatusUnauthorized {
					t.Errorf("anonymous: got %d, want 401", got.Code)
				}
				got := w.callWith(rt, body, w.who.customer.Cookies())
				if _, special := roleChecked[name]; special {
					// Refused by the role, or by the empty body that reaches the code first; never let in.
					if got.Code != http.StatusForbidden && got.Code != http.StatusBadRequest {
						t.Errorf("customer on a role-checked route: got %d, want 403 or 400", got.Code)
					}
				} else if !passed(got.Code) || got.Code >= 500 {
					t.Errorf("customer: got %d (%s)", got.Code, got.Body)
				}
			})
		case rt.Guard == apihttp.Sig:
			signed++
			t.Run(name, func(t *testing.T) {
				// A session is no substitute for the signature.
				for _, cookies := range [][]*http.Cookie{nil, w.who.admin.Cookies()} {
					if got := w.callWith(rt, body, cookies); got.Code < 400 || got.Code >= 500 {
						t.Errorf("unsigned call: got %d (%s)", got.Code, got.Body)
					}
				}
			})
		}
	}
	if public == 0 || user == 0 || signed == 0 {
		t.Fatalf("public %d, user %d, signed %d: the matrix is missing a kind of route", public, user, signed)
	}
	for name := range roleChecked {
		found := false
		for _, rt := range apihttp.Routes(w.a.Deps) {
			found = found || rt.Method+" "+rt.Pattern == name
		}
		if !found {
			t.Errorf("%s is listed as role-checked but is not a route", name)
		}
	}
}

// cookieOnly are state-changing routes whose whole input is the session cookie.
// Calling them twice would end the session, so they are not probed twice.
var cookieOnly = map[string]string{
	"POST /auth/logout":     "ends the session in the cookie",
	"POST /auth/refresh":    "reads the refresh cookie",
	"POST /auth/totp/setup": "starts the authenticator for the signed-in admin",
}

// whoMay picks a session that gets past the guard of the route, so the check
// reaches the code that reads the body.
func (w world) whoMay(rt apihttp.Route) []*http.Cookie {
	switch {
	case rt.Guard == apihttp.Admin:
		return w.who.admin.Cookies()
	case strings.HasPrefix(string(rt.Guard), "perm:"):
		perm := enum.Permission(strings.TrimPrefix(string(rt.Guard), "perm:"))
		for _, s := range []struct {
			session apptest.Session
			role    enum.Role
		}{{w.who.owner, enum.RoleShopOwner}, {w.who.staff, enum.RoleShopStaff}, {w.who.driver, enum.RoleShopDriver}, {w.who.platformDriver, enum.RolePlatformDriver}} {
			if (authz.Membership{Role: s.role}).Has(perm) {
				return s.session.Cookies()
			}
		}
	case rt.Pattern == "/auth/totp/setup" || rt.Pattern == "/auth/totp/verify":
		return w.who.adminNoTotp.Cookies()
	}
	return w.who.customer.Cookies()
}

// AUTHZ-05: a body may hold only the fields the route knows. Status, prices,
// totals, roles and ids of the shop or the user never come from the client.
//
// A route that reads a body refuses a stranger's field as VALIDATION_ERROR. A
// route that reads none (an action on the {id} in the path) must answer the same
// with or without the body, so nothing in it can be set from outside.
func TestAuthzMatrix_UnknownFieldsAreRefused(t *testing.T) {
	w := newWorld(t)
	evil := `{"status":"PAID","totalCents":1,"priceCents":1,"role":"SUPER_ADMIN","shopId":"` + uuid.NewString() + `","customerId":"` + uuid.NewString() + `","isAdmin":true}`
	type answer struct {
		Error struct{ Code string }
	}
	code := func(r *httptest.ResponseRecorder) (int, string) {
		var a answer
		_ = json.Unmarshal(r.Body.Bytes(), &a)
		return r.Code, a.Error.Code
	}
	readers, ignorers := 0, 0
	for _, rt := range apihttp.Routes(w.a.Deps) {
		name := rt.Method + " " + rt.Pattern
		if rt.Guard == apihttp.Sig || rt.Method == http.MethodGet || rt.Method == http.MethodHead {
			continue
		}
		if _, ok := cookieOnly[name]; ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			cookies := w.whoMay(rt)
			got := w.callWith(rt, evil, cookies)
			if status, c := code(got); status == http.StatusBadRequest && c == "VALIDATION_ERROR" && strings.Contains(got.Body.String(), "unknown_field") {
				readers++
				return
			}
			status, c := code(got)
			bare := w.callWith(rt, "", cookies)
			if bs, bc := code(bare); bs != status || bc != c {
				t.Errorf("with the extra fields: %d %s; without a body: %d %s (%s)", status, c, bs, bc, got.Body)
				return
			}
			ignorers++
		})
	}
	if readers < 40 || ignorers < 5 {
		t.Fatalf("%d routes refused unknown fields and %d ignore the body; the scan lost its way", readers, ignorers)
	}
}
