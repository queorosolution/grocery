package order_test

import (
	"context"
	"net/http"
	"testing"

	"gp/test/fakes"
)

const deleteBody = `{"confirm":"DELETE"}`

func (f fixture) userStatus() (status string) {
	f.a.T.Helper()
	if err := f.a.Env.Owner.QueryRow(context.Background(), `SELECT status FROM users WHERE email = 'nimal@example.com'`).Scan(&status); err != nil {
		f.a.T.Fatal(err)
	}
	return status
}

// FR-AUTH-09: an order in progress, even a bill nobody has paid, keeps the account open.
func TestDeleteAccount_RefusedWhileAnOrderIsOpen(t *testing.T) {
	f := newFixture(t)
	f.trio() // a bill sent to the shop, not yet answered
	e := f.me.Expect(http.MethodDelete, "/me", deleteBody, http.StatusConflict, "ACCOUNT_NOT_DELETABLE")
	if e.Error.Fields["orders"] != "open" {
		t.Errorf("fields = %v, want orders=open", e.Error.Fields)
	}
	if f.userStatus() != "ACTIVE" {
		t.Error("a refused delete changed the account")
	}
	f.me.OK(http.MethodGet, "/me", "") // still signed in
}

func TestDeleteAccount_RefusedWhileARefundIsUnpaid(t *testing.T) {
	f := newFixture(t)
	s := f.delivered()
	f.a.Exec(`INSERT INTO refunds (order_id, shop_id, type, amount_cents, reason, status)
		SELECT id, shop_id, 'FULL', 1000, 'Missing item', 'APPROVED' FROM orders WHERE id = $1`, s.id)
	e := f.me.Expect(http.MethodDelete, "/me", deleteBody, http.StatusConflict, "ACCOUNT_NOT_DELETABLE")
	if e.Error.Fields["refunds"] != "pending" || e.Error.Fields["orders"] != "" {
		t.Errorf("fields = %v, want only refunds=pending", e.Error.Fields)
	}
	f.a.Exec(`UPDATE refunds SET status = 'PAID_OUT', paid_out_at = now() WHERE order_id = $1`, s.id)
	f.me.OK(http.MethodDelete, "/me", deleteBody)
}

func TestDeleteAccount_RefusedForTeamMembersAndListsEveryReason(t *testing.T) {
	f := newFixture(t)
	f.trio()
	var uid string
	if err := f.a.Env.Owner.QueryRow(context.Background(), `SELECT id::text FROM users WHERE email = 'nimal@example.com'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	f.a.Exec(`INSERT INTO memberships (user_id, scope, shop_id, role) VALUES ($1, 'SHOP', $2, 'SHOP_STAFF')`, uid, f.shop)
	e := f.me.Expect(http.MethodDelete, "/me", deleteBody, http.StatusConflict, "ACCOUNT_NOT_DELETABLE")
	if e.Error.Fields["team"] != "member" || e.Error.Fields["orders"] != "open" {
		t.Errorf("fields = %v, want team=member and orders=open", e.Error.Fields)
	}
}

func TestDeleteAccount_NeedsTheWord(t *testing.T) {
	f := newFixture(t)
	for _, body := range []string{`{}`, `{"confirm":"delete"}`, `{"confirm":"yes"}`, `{"confirm":"DELETE","extra":1}`} {
		f.me.Expect(http.MethodDelete, "/me", body, http.StatusBadRequest, "VALIDATION_ERROR")
	}
	if f.userStatus() != "ACTIVE" {
		t.Error("the account was deleted without the word")
	}
}

// DATA-07: a finished account closes softly. History stays, access and personal details go.
func TestDeleteAccount_SoftDeleteKeepsHistoryAndEndsAccess(t *testing.T) {
	f := newFixture(t)
	s := f.delivered()
	before := f.a.Count(`SELECT count(*) FROM orders WHERE id = $1`, s.id)

	w := f.me.OK(http.MethodDelete, "/me", deleteBody)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", w.Code)
	}
	if f.userStatus() != "DELETED" {
		t.Errorf("status = %s, want DELETED (never a hard delete)", f.userStatus())
	}
	if f.a.Count(`SELECT count(*) FROM users WHERE email = 'nimal@example.com' AND name <> '' AND phone IS NULL AND phone_verified_at IS NULL`) != 1 {
		t.Error("name and e-mail must stay, the phone must go")
	}
	if n := f.a.Count(`SELECT count(*) FROM addresses a JOIN users u ON u.id = a.user_id WHERE u.email = 'nimal@example.com'`); n != 0 {
		t.Errorf("%d saved addresses remain", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM sessions s JOIN users u ON u.id = s.user_id WHERE u.email = 'nimal@example.com' AND s.revoked_at IS NULL`); n != 0 {
		t.Errorf("%d sessions still alive", n)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1`, s.id) != before {
		t.Error("the order history changed")
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'user.delete'`) != 1 {
		t.Error("no audit row for the delete")
	}

	// The person is out: no refresh, no new sign-in, and the old access token does nothing.
	if r := f.a.Call(http.MethodPost, "/v1/auth/refresh", "", f.me.Session.Refresh); r.Code != http.StatusUnauthorized {
		t.Errorf("refresh = %d, want 401", r.Code)
	}
	if r := f.a.SignIn(fakes.Claims{Sub: "nimal", Email: "nimal@example.com"}); r.Code != http.StatusForbidden {
		t.Errorf("sign-in = %d, want 403", r.Code)
	}
	f.me.Expect(http.MethodGet, "/me", "", http.StatusUnauthorized, "UNAUTHENTICATED")
	f.me.Expect(http.MethodPost, "/me/addresses", `{"label":"Home","line1":"12 Galle Road","lat":6.9,"lng":79.86}`, http.StatusUnauthorized, "UNAUTHENTICATED")
	// With the phone gone a bill is refused anyway; the user lock is the second wall.
	if r := f.submit(f.me, key1, f.body("")); r.Code < 400 {
		t.Errorf("a bill from a closed account = %d, want a refusal", r.Code)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id <> $1`, s.id) != 0 {
		t.Error("a closed account placed an order")
	}
}

func TestDeleteAccount_OtherPeopleAreUntouched(t *testing.T) {
	f := newFixture(t)
	other := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "kamal", Email: "kamal@example.com", Name: "Kamal"}))
	f.address(other)
	f.me.OK(http.MethodDelete, "/me", deleteBody)
	other.OK(http.MethodGet, "/me", "")
	if n := f.a.Count(`SELECT count(*) FROM addresses a JOIN users u ON u.id = a.user_id WHERE u.email = 'kamal@example.com'`); n != 1 {
		t.Errorf("another person's addresses = %d, want 1", n)
	}
}
