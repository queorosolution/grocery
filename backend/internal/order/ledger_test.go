package order_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

type ledgerRow struct {
	Type, Party string
	Amount      int64
}

func (f fixture) ledgerOf(id uuid.UUID) (rows []ledgerRow) {
	f.a.T.Helper()
	r, err := f.a.Env.Owner.Query(context.Background(), `SELECT type, party, amount_cents FROM ledger_entries WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		f.a.T.Fatal(err)
	}
	defer r.Close()
	for r.Next() {
		var l ledgerRow
		if err := r.Scan(&l.Type, &l.Party, &l.Amount); err != nil {
			f.a.T.Fatal(err)
		}
		rows = append(rows, l)
	}
	return rows
}

// checkPaid is docs/05 section 4: the rows of T10 are what the customer pays
// for the items and the service fee, less the discount, and the delivery fee is
// still to come. The shop's sale is what its own prices say for the lines left.
func (f fixture) checkPaid(id uuid.UUID, wantDiscountParty string) {
	f.a.T.Helper()
	rows := f.ledgerOf(id)
	var sum, margin int64
	by := map[string]ledgerRow{}
	for _, r := range rows {
		sum += r.Amount
		by[r.Type] = r
		if r.Type == "PLATFORM_MARGIN" {
			margin = r.Amount
		}
	}
	if want := int64(f.a.Count(`SELECT total_cents - delivery_fee_cents FROM orders WHERE id = $1`, id)); sum != want {
		f.a.T.Errorf("rows add up to %d, want total minus delivery fee = %d (%+v)", sum, want, rows)
	}
	sale := int64(f.a.Count(`SELECT COALESCE(sum(shop_price_cents * qty), 0)::bigint FROM order_items
		WHERE order_id = $1 AND state = 'ACTIVE' AND revision_no = (SELECT revision_no FROM orders WHERE id = $1)`, id))
	if s := by["SALE"]; s.Party != "SHOP" || s.Amount != sale {
		f.a.T.Errorf("SALE = %+v, want SHOP %d (shop prices of the lines left)", s, sale)
	}
	if want := int64(f.a.Count(`SELECT platform_margin_cents FROM orders WHERE id = $1`, id)); margin != want || by["PLATFORM_MARGIN"].Party != "PLATFORM" {
		f.a.T.Errorf("PLATFORM_MARGIN = %+v, want %d", by["PLATFORM_MARGIN"], want)
	}
	if d, ok := by["DISCOUNT"]; wantDiscountParty == "" && ok {
		f.a.T.Errorf("DISCOUNT row without a coupon: %+v", d)
	} else if wantDiscountParty != "" && (d.Party != wantDiscountParty || d.Amount >= 0) {
		f.a.T.Errorf("DISCOUNT = %+v, want a negative row for %s", d, wantDiscountParty)
	}
	if _, ok := by["DELIVERY_FEE"]; ok {
		f.a.T.Error("the delivery fee is posted on delivery, not on payment")
	}
}

func TestLedger_Paid_SumsToTotal(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		f := newFixture(t)
		x := f.unpaid()
		f.paid(x)
		f.checkPaid(x.o.ID, "")
	})
	t.Run("shop coupon", func(t *testing.T) {
		f := newFixture(t)
		x := f.unpaid()
		f.makeCoupon(f.admin(), "SHOP10")
		d := f.apply(f.me, x.o.ID, x.o.Version, "SHOP10")
		if d.Code != http.StatusOK {
			t.Fatalf("apply = %d %s", d.Code, d.Body)
		}
		x.o = f.get(f.me, x.o.ID)
		f.paid(x)
		f.checkPaid(x.o.ID, "SHOP")
		if f.a.Count(`SELECT count(*) FROM coupon_redemptions WHERE order_id = $1 AND confirmed_at IS NOT NULL`, x.o.ID) != 1 {
			t.Error("the coupon use was not confirmed by the payment")
		}
	})
	t.Run("platform coupon", func(t *testing.T) {
		f := newFixture(t)
		x := f.unpaid()
		f.makeCoupon(f.admin(), "PLAT5", "fundedBy", "PLATFORM", "type", "FIXED", "value", 3000, "shopId", nil)
		if w := f.apply(f.me, x.o.ID, x.o.Version, "PLAT5"); w.Code != http.StatusOK {
			t.Fatalf("apply = %d %s", w.Code, w.Body)
		}
		x.o = f.get(f.me, x.o.ID)
		f.paid(x)
		f.checkPaid(x.o.ID, "PLATFORM")
	})
	t.Run("revised bill", func(t *testing.T) {
		f := newFixture(t)
		f.usePayHere()
		x := f.trio()
		staff := f.staff("asha", "SHOP_STAFF")
		f.review(staff, x.o.ID, version(1, changes(qtyOf(x.rice0.ID, 2), removeOf(x.sug0.ID))))
		f.me.OK(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":2,"revisionNo":1}`)
		x.o = f.get(f.me, x.o.ID)
		f.paid(x)
		f.checkPaid(x.o.ID, "")
	})
}

// The ledger is append-only, even for the database owner (docs/05 section 3).
func TestLedger_AppendOnly(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	f.paid(x)
	for _, q := range []string{`UPDATE ledger_entries SET amount_cents = 1`, `DELETE FROM ledger_entries`} {
		if _, err := f.a.Env.Owner.Exec(context.Background(), q); err == nil {
			t.Errorf("%q was allowed", q)
		}
	}
}

// A shop sees its own ledger rows only.
func TestLedger_Isolation(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	f.paid(x)
	other, _ := f.readyShop("other-shop")
	var seen int
	err := f.a.Env.DB.Tx(context.Background(), db.Scope{UserID: uuid.New(), ShopID: other, Role: enum.RoleShopOwner}, func(ctx context.Context, tx *db.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM ledger_entries`).Scan(&seen)
	})
	if err != nil || seen != 0 {
		t.Errorf("another shop sees %d ledger rows (err %v)", seen, err)
	}
}
