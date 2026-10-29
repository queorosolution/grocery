package user_test

import (
	"net/http"
	"strings"
	"testing"

	"gp/internal/auth"
	"gp/test/apptest"
)

func TestSetName(t *testing.T) {
	f := newFixture(t)
	me := apptest.Decode[auth.Me](t, f.me.OK(http.MethodPatch, "/me", `{"name":"  Nimal Perera "}`))
	if me.Name != "Nimal Perera" || me.Email != "nimal@example.com" {
		t.Fatalf("me = %+v", me)
	}
	if n := f.a.Count(`SELECT count(*) FROM users WHERE name = 'Nimal Perera'`); n != 1 {
		t.Errorf("stored name not trimmed: %d rows", n)
	}
	for name, body := range map[string]string{
		"empty":        `{"name":""}`,
		"spaces":       `{"name":"   "}`,
		"too long":     `{"name":"` + strings.Repeat("a", 81) + `"}`,
		"control char": `{"name":"a\u0000b"}`,
		"other field":  `{"name":"x","email":"boss@example.com"}`,
		"status":       `{"name":"x","status":"ACTIVE"}`,
	} {
		f.me.Expect(http.MethodPatch, "/me", body, http.StatusBadRequest, "VALIDATION_ERROR")
		_ = name
	}
	if n := f.a.Count(`SELECT count(*) FROM users WHERE email = 'nimal@example.com' AND name = 'Nimal Perera'`); n != 1 {
		t.Error("a refused change must keep the old name")
	}
}

// A name change only ever touches the caller's own row, whatever the body says.
func TestSetNameOnlyChangesTheCaller(t *testing.T) {
	f := newFixture(t)
	other := f.another("kamal")
	other.OK(http.MethodPatch, "/me", `{"name":"Kamal"}`)
	f.me.OK(http.MethodPatch, "/me", `{"name":"Nimal 2"}`)
	if n := f.a.Count(`SELECT count(*) FROM users WHERE name = 'Kamal'`); n != 1 {
		t.Error("the other customer's name changed")
	}
}

func TestMeRoutesNeedSignIn(t *testing.T) {
	a := apptest.New(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodPatch, "/v1/me"}, {http.MethodPost, "/v1/me/phone/send"}, {http.MethodPost, "/v1/me/phone/verify"},
		{http.MethodGet, "/v1/me/addresses"}, {http.MethodPost, "/v1/me/addresses"},
		{http.MethodPatch, "/v1/me/addresses/" + "3f2b8f0e-0000-4000-8000-000000000000"},
		{http.MethodDelete, "/v1/me/addresses/" + "3f2b8f0e-0000-4000-8000-000000000000"},
	} {
		if w := a.Call(c.method, c.path, `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c.method, c.path, w.Code)
		}
	}
}
