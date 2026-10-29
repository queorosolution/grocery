package auth_test

import (
	"net/http"
	"testing"

	"gp/internal/auth"
	"gp/test/apptest"
	"gp/test/fakes"
)

type userList struct{ Data []auth.AdminUser }

func TestAdminSearchUsers(t *testing.T) {
	a := apptest.New(t)
	a.SignedIn(fakes.Claims{Sub: "a", Email: "amal@example.test", Name: "Amal Perera"})
	a.SignedIn(fakes.Claims{Sub: "b", Email: "bimal@example.test", Name: "Bimal"})
	admin := a.Admin()

	list := func(q string) []auth.AdminUser {
		w := a.Call(http.MethodGet, "/v1/admin/users"+q, "", admin.Cookies()...)
		if w.Code != http.StatusOK {
			t.Fatalf("list %q = %d %s", q, w.Code, w.Body)
		}
		return apptest.Decode[userList](t, w).Data
	}
	if got := list(""); len(got) != 3 {
		t.Errorf("all users = %d, want 3", len(got))
	}
	if got := list("?q=AMAL"); len(got) != 1 || got[0].Email != "amal@example.test" {
		t.Errorf("q=AMAL = %+v", got)
	}
	if got := list("?q=perera"); len(got) != 0 {
		t.Errorf("a surname is not a prefix match, got %+v", got)
	}
	// LIKE wildcards in the search text are plain characters.
	if got := list("?q=%25"); len(got) != 0 {
		t.Errorf("q=%%25 matched %d users", len(got))
	}
}

// AUTHN-08: blocking ends access at once and keeps the user out.
func TestAdminBlockUser(t *testing.T) {
	a := apptest.New(t)
	user := a.SignedIn(fakes.Claims{Sub: "u", Email: "u@example.test"})
	admin := a.Admin()
	patch := func(id, body string) int {
		return a.Call(http.MethodPatch, "/v1/admin/users/"+id, body, admin.Cookies()...).Code
	}

	if got := patch(user.Me.ID.String(), `{"status":"BLOCKED"}`); got != http.StatusNoContent {
		t.Fatalf("block = %d", got)
	}
	if w := a.Call(http.MethodPost, "/v1/auth/refresh", "", user.Refresh); w.Code != http.StatusUnauthorized {
		t.Errorf("refresh after block = %d, want 401", w.Code)
	}
	if w := a.Call(http.MethodGet, "/v1/me", "", user.Access); w.Code != http.StatusUnauthorized {
		t.Errorf("/me with the old access token = %d, want 401", w.Code)
	}
	if w := a.SignIn(fakes.Claims{Sub: "u", Email: "u@example.test"}); w.Code != http.StatusForbidden {
		t.Errorf("sign-in after block = %d, want 403", w.Code)
	}
	if got := patch(user.Me.ID.String(), `{"status":"ACTIVE"}`); got != http.StatusNoContent {
		t.Fatalf("unblock = %d", got)
	}
	if w := a.SignIn(fakes.Claims{Sub: "u", Email: "u@example.test"}); w.Code != http.StatusOK {
		t.Errorf("sign-in after unblock = %d", w.Code)
	}
}

func TestAdminUserStatusRules(t *testing.T) {
	a := apptest.New(t)
	user := a.SignedIn(fakes.Claims{Sub: "u", Email: "u@example.test"})
	admin := a.Admin()
	patch := func(id, body string, cs ...*http.Cookie) int {
		return a.Call(http.MethodPatch, "/v1/admin/users/"+id, body, cs...).Code
	}
	for name, c := range map[string]struct {
		id, body string
		cookies  []*http.Cookie
		want     int
	}{
		"block yourself":      {admin.Me.ID.String(), `{"status":"BLOCKED"}`, admin.Cookies(), http.StatusForbidden},
		"set DELETED":         {user.Me.ID.String(), `{"status":"DELETED"}`, admin.Cookies(), http.StatusBadRequest},
		"unknown status":      {user.Me.ID.String(), `{"status":"X"}`, admin.Cookies(), http.StatusBadRequest},
		"unknown field":       {user.Me.ID.String(), `{"status":"BLOCKED","role":"SUPER_ADMIN"}`, admin.Cookies(), http.StatusBadRequest},
		"unknown user":        {"00000000-0000-0000-0000-000000000001", `{"status":"BLOCKED"}`, admin.Cookies(), http.StatusNotFound},
		"bad id":              {"nope", `{"status":"BLOCKED"}`, admin.Cookies(), http.StatusNotFound},
		"customer is refused": {admin.Me.ID.String(), `{"status":"BLOCKED"}`, user.Cookies(), http.StatusForbidden},
	} {
		if got := patch(c.id, c.body, c.cookies...); got != c.want {
			t.Errorf("%s = %d, want %d", name, got, c.want)
		}
	}
}
