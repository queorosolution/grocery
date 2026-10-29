package team_test

import (
	"net/http"
	"testing"
	"time"

	"gp/internal/auth"
	"gp/internal/shared/clock"
	"gp/internal/team"
	"gp/test/apptest"
	"gp/test/fakes"
)

const inviteeEmail = "invitee@example.test"

func (f fixture) invite(role string) team.Issued {
	f.a.T.Helper()
	w := f.a.Call(http.MethodPost, f.teamURL(f.shopA, "/invites"), `{"email":"`+inviteeEmail+`","role":"`+role+`"}`, f.owner.Cookies()...)
	if w.Code != http.StatusCreated {
		f.a.T.Fatalf("invite = %d %s", w.Code, w.Body)
	}
	return apptest.Decode[team.Issued](f.a.T, w)
}

// AUTHN-07: the link works once, for the invited Google account only.
func TestAcceptInvite(t *testing.T) {
	f := newFixture(t)
	inv := f.invite("SHOP_STAFF")

	// Another Google account cannot use it, and the link still works afterwards.
	w := f.a.SignInWithInvite(fakes.Claims{Sub: "thief", Email: "thief@example.test"}, inv.Token)
	if w.Code != http.StatusForbidden {
		t.Fatalf("other account = %d, want 403", w.Code)
	}
	if n := f.a.Count(`SELECT count(*) FROM users WHERE google_sub = 'thief'`); n != 0 {
		t.Fatal("a failed accept left a user behind")
	}

	w = f.a.SignInWithInvite(fakes.Claims{Sub: "invitee", Email: "Invitee@Example.test"}, inv.Token)
	if w.Code != http.StatusOK {
		t.Fatalf("invited account = %d %s", w.Code, w.Body)
	}
	me := apptest.Decode[auth.Me](t, w)
	if len(me.Roles) != 1 || me.Roles[0].Role != "SHOP_STAFF" || me.Roles[0].ShopSlug == nil || *me.Roles[0].ShopSlug != "alpha" {
		t.Fatalf("roles = %+v", me.Roles)
	}
	// The new member can now use the console (staff: 403 on team, 404 on shop B).
	cs := apptest.Cookies(w)
	at := cs[auth.AccessCookie]
	if w := f.a.Call(http.MethodGet, f.teamURL(f.shopA, ""), "", at); w.Code != http.StatusForbidden {
		t.Errorf("new staff on team = %d, want 403", w.Code)
	}
	if w := f.a.Call(http.MethodGet, f.teamURL(f.shopB, ""), "", at); w.Code != http.StatusNotFound {
		t.Errorf("new staff on shop B = %d, want 404", w.Code)
	}

	// Second use: gone.
	w = f.a.SignInWithInvite(fakes.Claims{Sub: "invitee", Email: inviteeEmail}, inv.Token)
	if w.Code != http.StatusNotFound {
		t.Fatalf("second accept = %d, want 404", w.Code)
	}
	if n := f.a.Count(`SELECT count(*) FROM memberships m JOIN users u ON u.id = m.user_id WHERE u.google_sub = 'invitee'`); n != 1 {
		t.Fatalf("memberships = %d, want 1", n)
	}
}

func TestInviteNotUsable(t *testing.T) {
	f := newFixture(t)
	claims := fakes.Claims{Sub: "invitee", Email: inviteeEmail}

	if w := f.a.SignInWithInvite(claims, "never-issued"); w.Code != http.StatusNotFound {
		t.Errorf("unknown token = %d, want 404", w.Code)
	}

	// A newer invite for the same e-mail and shop replaces the older link.
	old := f.invite("SHOP_STAFF")
	fresh := f.invite("SHOP_DRIVER")
	if w := f.a.SignInWithInvite(claims, old.Token); w.Code != http.StatusNotFound {
		t.Errorf("replaced invite = %d, want 404", w.Code)
	}

	// Seven days pass.
	restore := clock.Freeze(clock.Now().Add(team.InviteTTL + time.Minute))
	w := f.a.SignInWithInvite(claims, fresh.Token)
	restore()
	if w.Code != http.StatusNotFound {
		t.Errorf("expired invite = %d, want 404", w.Code)
	}

	// Sign-in without a token still works for the same person.
	if w := f.a.SignIn(claims); w.Code != http.StatusOK {
		t.Errorf("plain sign-in = %d", w.Code)
	}
}

func TestInviteTokenLength(t *testing.T) {
	f := newFixture(t)
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	if w := f.a.SignInWithInvite(fakes.Claims{Sub: "x", Email: "x@example.test"}, string(long)); w.Code != http.StatusBadRequest {
		t.Fatalf("oversized token = %d, want 400", w.Code)
	}
}

func TestAdminInvites(t *testing.T) {
	f := newFixture(t)
	admin := f.a.Admin()
	post := func(body string) int {
		return f.a.Call(http.MethodPost, "/v1/admin/invites", body, admin.Cookies()...).Code
	}
	shop := f.shopA.String()

	// Owners need a shop, platform drivers must not have one.
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"owner of a shop":        {`{"email":"o@example.test","role":"SHOP_OWNER","shopId":"` + shop + `"}`, http.StatusCreated},
		"platform driver":        {`{"email":"d@example.test","role":"PLATFORM_DRIVER"}`, http.StatusCreated},
		"owner without a shop":   {`{"email":"o@example.test","role":"SHOP_OWNER"}`, http.StatusBadRequest},
		"driver with a shop":     {`{"email":"d@example.test","role":"PLATFORM_DRIVER","shopId":"` + shop + `"}`, http.StatusBadRequest},
		"unknown shop":           {`{"email":"o@example.test","role":"SHOP_OWNER","shopId":"00000000-0000-0000-0000-000000000001"}`, http.StatusBadRequest},
		"super admin by invite":  {`{"email":"o@example.test","role":"SUPER_ADMIN"}`, http.StatusBadRequest},
		"role in a console-only": {`{"email":"o@example.test","role":"CUSTOMER","shopId":"` + shop + `"}`, http.StatusBadRequest},
	} {
		if got := post(c.body); got != c.want {
			t.Errorf("%s = %d, want %d", name, got, c.want)
		}
	}

	// A platform driver invite becomes a platform membership.
	w := f.a.Call(http.MethodPost, "/v1/admin/invites", `{"email":"`+inviteeEmail+`","role":"PLATFORM_DRIVER"}`, admin.Cookies()...)
	inv := apptest.Decode[team.Issued](t, w)
	me := apptest.Decode[auth.Me](t, f.a.SignInWithInvite(fakes.Claims{Sub: "pd", Email: inviteeEmail}, inv.Token))
	if len(me.Roles) != 1 || me.Roles[0].Role != "PLATFORM_DRIVER" || me.Roles[0].ShopID != nil {
		t.Fatalf("roles = %+v", me.Roles)
	}

	// Revoking: the link stops working and a second revoke is 404.
	w = f.a.Call(http.MethodPost, "/v1/admin/invites", `{"email":"later@example.test","role":"PLATFORM_DRIVER"}`, admin.Cookies()...)
	inv = apptest.Decode[team.Issued](t, w)
	del := func() int {
		return f.a.Call(http.MethodDelete, "/v1/admin/invites/"+inv.Invite.ID.String(), "", admin.Cookies()...).Code
	}
	if got := del(); got != http.StatusNoContent {
		t.Fatalf("revoke = %d", got)
	}
	if got := del(); got != http.StatusNotFound {
		t.Errorf("revoke again = %d, want 404", got)
	}
	if w := f.a.SignInWithInvite(fakes.Claims{Sub: "later", Email: "later@example.test"}, inv.Token); w.Code != http.StatusNotFound {
		t.Errorf("revoked invite = %d, want 404", w.Code)
	}
}
