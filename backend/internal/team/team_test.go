package team_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/team"
	"gp/test/apptest"
	"gp/test/fakes"
)

type fixture struct {
	a            *apptest.App
	shopA, shopB uuid.UUID
	owner, staff apptest.Session // of shop A
	otherOwner   apptest.Session // owner of shop B
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	a := apptest.New(t)
	f := fixture{a: a, shopA: a.SeedShop("alpha"), shopB: a.SeedShop("beta")}
	f.owner = a.SignedIn(fakes.Claims{Sub: "owner", Email: "owner@example.test", Name: "Owner"})
	f.staff = a.SignedIn(fakes.Claims{Sub: "staff", Email: "staff@example.test", Name: "Staff"})
	f.otherOwner = a.SignedIn(fakes.Claims{Sub: "owner-b", Email: "owner-b@example.test", Name: "Owner B"})
	a.Grant(f.owner.Me.ID, &f.shopA, "SHOP_OWNER")
	a.Grant(f.staff.Me.ID, &f.shopA, "SHOP_STAFF")
	a.Grant(f.otherOwner.Me.ID, &f.shopB, "SHOP_OWNER")
	return f
}

func (f fixture) teamURL(shop uuid.UUID, rest string) string {
	return "/v1/console/shops/" + shop.String() + "/team" + rest
}

// membershipOf finds the membership id of a user in shop A.
func (f fixture) membershipOf(user uuid.UUID) uuid.UUID {
	var id uuid.UUID
	if err := f.a.Env.Owner.QueryRow(f.a.T.Context(),
		`SELECT id FROM memberships WHERE user_id = $1 AND shop_id = $2`, user, f.shopA).Scan(&id); err != nil {
		f.a.T.Fatal(err)
	}
	return id
}

func TestTeamListShowsMembersAndInvites(t *testing.T) {
	f := newFixture(t)
	f.a.Call(http.MethodPost, f.teamURL(f.shopA, "/invites"), `{"email":"new@example.test","role":"SHOP_DRIVER"}`, f.owner.Cookies()...)

	w := f.a.Call(http.MethodGet, f.teamURL(f.shopA, ""), "", f.owner.Cookies()...)
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d %s", w.Code, w.Body)
	}
	got := apptest.Decode[team.Team](t, w)
	if len(got.Members) != 2 || len(got.Invites) != 1 || got.Invites[0].Email != "new@example.test" {
		t.Fatalf("team = %+v", got)
	}
	// Owners see names and e-mails of their own team only.
	for _, m := range got.Members {
		if m.Email == "owner-b@example.test" {
			t.Fatal("another shop's member is listed")
		}
	}
}

// AUTHZ-04: staff cannot manage the team, and owners cannot create owners.
func TestInviteRules(t *testing.T) {
	f := newFixture(t)
	body := func(role string) string { return `{"email":"new@example.test","role":"` + role + `"}` }

	if w := f.a.Call(http.MethodPost, f.teamURL(f.shopA, "/invites"), body("SHOP_STAFF"), f.staff.Cookies()...); w.Code != http.StatusForbidden {
		t.Errorf("staff invite = %d, want 403", w.Code)
	}
	for _, role := range []string{"SHOP_OWNER", "PLATFORM_DRIVER", "SUPER_ADMIN", "NOPE"} {
		if w := f.a.Call(http.MethodPost, f.teamURL(f.shopA, "/invites"), body(role), f.owner.Cookies()...); w.Code != http.StatusBadRequest {
			t.Errorf("owner invites %s = %d, want 400", role, w.Code)
		}
	}
	for name, b := range map[string]string{
		"bad email":     `{"email":"not-an-email","role":"SHOP_STAFF"}`,
		"unknown field": `{"email":"a@example.test","role":"SHOP_STAFF","shopId":"` + f.shopB.String() + `"}`,
	} {
		if w := f.a.Call(http.MethodPost, f.teamURL(f.shopA, "/invites"), b, f.owner.Cookies()...); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, w.Code)
		}
	}

	w := f.a.Call(http.MethodPost, f.teamURL(f.shopA, "/invites"), `{"email":"New@Example.test","role":"SHOP_STAFF","canEditProducts":true}`, f.owner.Cookies()...)
	if w.Code != http.StatusCreated {
		t.Fatalf("owner invites staff = %d %s", w.Code, w.Body)
	}
	got := apptest.Decode[team.Issued](t, w)
	if got.Token == "" || got.Invite.Email != "new@example.test" || !got.Invite.CanEditProducts || got.Invite.ExpiresAt.IsZero() {
		t.Fatalf("issued = %+v", got)
	}
	// Only a hash of the token is stored.
	if n := f.a.Count(`SELECT count(*) FROM invites WHERE token_hash = $1`, []byte(got.Token)); n != 0 {
		t.Fatal("the invite token is stored in clear")
	}
}

func TestChangeAndRemoveRules(t *testing.T) {
	f := newFixture(t)
	ownerID, staffID := f.membershipOf(f.owner.Me.ID), f.membershipOf(f.staff.Me.ID)
	url := func(id uuid.UUID) string { return f.teamURL(f.shopA, "/"+id.String()) }

	// Not allowed: staff editing, an owner editing themselves or another owner.
	if w := f.a.Call(http.MethodPatch, url(staffID), `{"canEditProducts":true}`, f.staff.Cookies()...); w.Code != http.StatusForbidden {
		t.Errorf("staff changes a member = %d, want 403", w.Code)
	}
	if w := f.a.Call(http.MethodPatch, url(ownerID), `{"role":"SHOP_STAFF"}`, f.owner.Cookies()...); w.Code != http.StatusForbidden {
		t.Errorf("owner changes own role = %d, want 403", w.Code)
	}
	if w := f.a.Call(http.MethodPatch, url(staffID), `{"role":"SHOP_OWNER"}`, f.owner.Cookies()...); w.Code != http.StatusBadRequest {
		t.Errorf("promote to owner = %d, want 400", w.Code)
	}
	if w := f.a.Call(http.MethodPatch, url(staffID), `{}`, f.owner.Cookies()...); w.Code != http.StatusBadRequest {
		t.Errorf("empty change = %d, want 400", w.Code)
	}

	// Allowed: the flag, then staff to driver (which clears the flag).
	if w := f.a.Call(http.MethodPatch, url(staffID), `{"canEditProducts":true}`, f.owner.Cookies()...); w.Code != http.StatusNoContent {
		t.Fatalf("grant product edit = %d %s", w.Code, w.Body)
	}
	if n := f.a.Count(`SELECT count(*) FROM memberships WHERE id = $1 AND can_edit_products`, staffID); n != 1 {
		t.Fatal("flag not saved")
	}
	if w := f.a.Call(http.MethodPatch, url(staffID), `{"role":"SHOP_DRIVER"}`, f.owner.Cookies()...); w.Code != http.StatusNoContent {
		t.Fatalf("staff to driver = %d %s", w.Code, w.Body)
	}
	if n := f.a.Count(`SELECT count(*) FROM memberships WHERE id = $1 AND role = 'SHOP_DRIVER' AND NOT can_edit_products`, staffID); n != 1 {
		t.Fatal("role not changed or flag not cleared")
	}

	// The last owner cannot go; a second owner makes it possible.
	if w := f.a.Call(http.MethodDelete, url(ownerID), "", f.owner.Cookies()...); w.Code != http.StatusConflict {
		t.Errorf("remove last owner = %d, want 409", w.Code)
	}
	second := f.a.SignedIn(fakes.Claims{Sub: "owner2", Email: "owner2@example.test"})
	f.a.Grant(second.Me.ID, &f.shopA, "SHOP_OWNER")
	if w := f.a.Call(http.MethodDelete, url(ownerID), "", second.Cookies()...); w.Code != http.StatusNoContent {
		t.Errorf("remove one of two owners = %d, want 204", w.Code)
	}
	// Removal acts at once: the old cookies lose access on the next request.
	if w := f.a.Call(http.MethodGet, f.teamURL(f.shopA, ""), "", f.owner.Cookies()...); w.Code != http.StatusNotFound {
		t.Errorf("removed owner still reaches the team = %d, want 404", w.Code)
	}
}

// AUTHZ-02 (team routes): a member of shop B gets 404 on every shop A route.
func TestOtherShopGets404(t *testing.T) {
	f := newFixture(t)
	staffID := f.membershipOf(f.staff.Me.ID)
	calls := []struct{ method, path, body string }{
		{http.MethodGet, f.teamURL(f.shopA, ""), ""},
		{http.MethodPost, f.teamURL(f.shopA, "/invites"), `{"email":"x@example.test","role":"SHOP_STAFF"}`},
		{http.MethodPatch, f.teamURL(f.shopA, "/"+staffID.String()), `{"canEditProducts":true}`},
		{http.MethodDelete, f.teamURL(f.shopA, "/"+staffID.String()), ""},
		{http.MethodGet, "/v1/console/shops/not-a-uuid/team", ""},
	}
	for _, c := range calls {
		if w := f.a.Call(c.method, c.path, c.body, f.otherOwner.Cookies()...); w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", c.method, c.path, w.Code)
		}
	}
	// The same member under their own shop's id cannot touch a membership of shop A.
	if w := f.a.Call(http.MethodDelete, f.teamURL(f.shopB, "/"+staffID.String()), "", f.otherOwner.Cookies()...); w.Code != http.StatusNotFound {
		t.Errorf("remove a foreign membership = %d, want 404", w.Code)
	}
	if n := f.a.Count(`SELECT count(*) FROM memberships WHERE id = $1 AND status = 'ACTIVE'`, staffID); n != 1 {
		t.Fatal("the foreign membership was changed")
	}
}
