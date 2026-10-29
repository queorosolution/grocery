package order_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/finance"
	"gp/internal/shared/clock"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

func today() string { return clock.Colombo(clock.Now()).Format(time.DateOnly) }

func (f fixture) shopBalance() int64 {
	return int64(f.a.Count(`SELECT COALESCE(sum(amount_cents), 0) FROM ledger_entries WHERE shop_id = $1 AND party = 'SHOP'`, f.shop))
}

func (f fixture) createPayout(from, to string) *http.Response {
	return f.admin().Call(http.MethodPost, "/admin/payouts", `{"shopId":"`+f.shop.String()+`","from":"`+from+`","to":"`+to+`"}`).Result()
}

// delivered is an order that went through the whole flow, so the shop is owed its sale and the delivery fee.
func (f fixture) delivered() shopFlow {
	f.a.T.Helper()
	s := f.outForDelivery()
	s.driver.OK(http.MethodPost, driverPath(s.id, "/complete"), version(f.get(f.me, s.id).Version, `"code":"`+f.code(s.id)+`"`))
	return s
}

// FR-FIN-03: the amount comes from the ledger, and a paid payout takes it off the balance.
func TestPayout_AmountFromLedger(t *testing.T) {
	f := newFixture(t)
	f.delivered()
	owed := f.shopBalance()
	if owed <= 0 {
		t.Fatalf("the shop is owed %d", owed)
	}
	admin := f.admin()
	resp := f.createPayout(today(), today())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	if n := f.a.Count(`SELECT count(*) FROM payouts WHERE shop_id = $1 AND status = 'DRAFT' AND amount_cents = $2`, f.shop, owed); n != 1 {
		t.Fatalf("no draft of %d", owed)
	}
	var pid uuid.UUID
	if err := f.a.Env.Owner.QueryRow(t.Context(), `SELECT id FROM payouts WHERE shop_id = $1`, f.shop).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if f.shopBalance() != owed {
		t.Error("a draft changed the balance")
	}
	if f.createPayout(today(), today()).StatusCode != http.StatusConflict {
		t.Error("a second draft was made")
	}
	admin.Expect(http.MethodPost, "/admin/payouts/"+pid.String()+"/paid", `{"reference":"x"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	paid := apptest.Decode[finance.Payout](t, admin.OK(http.MethodPost, "/admin/payouts/"+pid.String()+"/paid", `{"reference":"BANK-PO-1"}`))
	if paid.Status != "PAID" || paid.PaidAt == nil || int64(paid.AmountCents) != owed {
		t.Fatalf("paid = %+v", paid)
	}
	if f.shopBalance() != 0 || f.a.Count(`SELECT count(*) FROM ledger_entries WHERE payout_id = $1 AND type = 'PAYOUT' AND party = 'SHOP' AND amount_cents = $2`, pid, -owed) != 1 {
		t.Errorf("balance %d after the payout", f.shopBalance())
	}
	admin.Expect(http.MethodPost, "/admin/payouts/"+pid.String()+"/paid", `{"reference":"BANK-PO-1"}`, http.StatusConflict, "CONFLICT")
	// The same days again overlap the payout; nothing is due anyway.
	admin.Expect(http.MethodPost, "/admin/payouts", `{"shopId":"`+f.shop.String()+`","from":"`+today()+`","to":"`+today()+`"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action IN ('payout.create', 'payout.paid')`) != 2 {
		t.Error("the payout is not audited")
	}
}

// A refund paid out lowers what the shop is owed; the payout follows the ledger.
func TestPayout_FollowsRefunds(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	r := f.fullRequest(s.staff, s.id)
	f.decide(r.ID, true, "")
	f.paidOut(r.ID, "BANK-REF-90")
	if f.shopBalance() != 0 {
		t.Errorf("balance after a full refund = %d, want 0", f.shopBalance())
	}
	f.admin().Expect(http.MethodPost, "/admin/payouts", `{"shopId":"`+f.shop.String()+`","from":"`+today()+`","to":"`+today()+`"}`, http.StatusConflict, "CONFLICT")
}

func TestPayout_Validation(t *testing.T) {
	f := newFixture(t)
	a := f.admin()
	tomorrow := clock.Colombo(clock.Now()).AddDate(0, 0, 1).Format(time.DateOnly)
	for name, body := range map[string]string{
		"future":    `{"shopId":"` + f.shop.String() + `","from":"` + today() + `","to":"` + tomorrow + `"}`,
		"bad date":  `{"shopId":"` + f.shop.String() + `","from":"1/2/2026","to":"` + today() + `"}`,
		"reversed":  `{"shopId":"` + f.shop.String() + `","from":"` + today() + `","to":"2020-01-01"}`,
		"no shop":   `{"from":"` + today() + `","to":"` + today() + `"}`,
		"not found": `{"shopId":"` + uuid.NewString() + `","from":"` + today() + `","to":"` + today() + `"}`,
	} {
		if resp := a.Call(http.MethodPost, "/admin/payouts", body).Result(); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
	a.Expect(http.MethodPost, "/admin/payouts", `{"shopId":"`+f.shop.String()+`","from":"`+today()+`","to":"`+today()+`"}`, http.StatusConflict, "CONFLICT") // nothing is due
	a.Expect(http.MethodGet, "/admin/payouts?shopId=junk", "", http.StatusBadRequest, "VALIDATION_ERROR")
	a.Expect(http.MethodPost, "/admin/payouts/"+uuid.NewString()+"/paid", `{"reference":"BANK-PO-2"}`, http.StatusNotFound, "NOT_FOUND")
}

// The owner sees the balance and the payouts of their own shop only; staff do not.
func TestPayout_ShopView(t *testing.T) {
	f := newFixture(t)
	f.delivered()
	owner := f.staff("owner1", string(enum.RoleShopOwner))
	staff := f.staff("staff1", string(enum.RoleShopStaff))
	path := "/console/shops/" + f.shop.String() + "/payouts"
	f.createPayout(today(), today())

	view := apptest.Decode[finance.ShopView](t, owner.OK(http.MethodGet, path, ""))
	if int64(view.BalanceCents) != f.shopBalance() || len(view.Data) != 1 || view.Data[0].Status != "DRAFT" {
		t.Errorf("view = %+v", view)
	}
	staff.Expect(http.MethodGet, path, "", http.StatusForbidden, "FORBIDDEN")
	other, _ := f.readyShop("other")
	owner.Expect(http.MethodGet, "/console/shops/"+other.String()+"/payouts", "", http.StatusNotFound, "NOT_FOUND")
	f.me.Expect(http.MethodGet, "/admin/payouts", "", http.StatusForbidden, "FORBIDDEN")
	list := apptest.Decode[struct{ Data []finance.Payout }](t, f.admin().OK(http.MethodGet, "/admin/payouts?shopId="+f.shop.String(), ""))
	if len(list.Data) != 1 {
		t.Errorf("admin list = %d", len(list.Data))
	}
}
