package connector_test

import (
	"context"
	"net/http"
	"testing"

	"gp/internal/connector"
	"gp/internal/shared/enum"
)

func (r *rig) openAlways() {
	for day := 1; day <= 7; day++ {
		r.a.Exec(`INSERT INTO shop_hours (shop_id, weekday, opens, closes) VALUES ($1, $2, '00:00', '24:00')`, r.shopID, day)
	}
}

// run does what the sync function does with a job.
func (r *rig) run(c connector.Connector, trigger enum.RunTrigger, ids ...string) (connector.RunResult, error) {
	r.a.T.Helper()
	return r.a.Connector.Run(context.Background(), connector.Job{ConnectorID: c.ID, Trigger: trigger, ExternalIDs: ids})
}

func (r *rig) mustRun(c connector.Connector, trigger enum.RunTrigger, ids ...string) connector.RunResult {
	r.a.T.Helper()
	res, err := r.run(c, trigger, ids...)
	if err != nil {
		r.a.T.Fatalf("run: %v", err)
	}
	return res
}

type sold struct {
	ShopPrice int64
	Available bool
	Managed   string
	Found     bool
}

func (r *rig) sold(ext string) sold {
	r.a.T.Helper()
	var s sold
	rows, err := r.a.Env.Owner.Query(context.Background(),
		`SELECT shop_price_cents, is_available, managed_by FROM shop_products WHERE shop_id = $1 AND external_id = $2`, r.shopID, ext)
	if err != nil {
		r.a.T.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		s.Found = true
		if err := rows.Scan(&s.ShopPrice, &s.Available, &s.Managed); err != nil {
			r.a.T.Fatal(err)
		}
	}
	return s
}

func (r *rig) openIssues(kind string) int {
	return r.a.Count(`SELECT count(*) FROM sync_issues WHERE status = 'OPEN' AND kind = $1`, kind)
}

// seedFull is the shop of the preview test: four products already sold, one master product with a barcode.
func seedFull(r *rig) {
	seedPreview(r)
	r.source("A1", str("Rice"), nil, 105, 0)                    // update
	r.source("A2", str("Sugar"), nil, 40, 0)                    // more than half down
	r.source("A3", str("Tea"), nil, 130, 120)                   // above the MRP
	r.source("A4", str("Salt"), nil, 0, 0)                      // zero price
	r.source("B1", str("Biscuits"), str(barcodeKnown), 90, 100) // linked by barcode
	r.source("B2", str("Loose item"), nil, 50, 0)               // nothing to match by
	r.source("B3", nil, nil, 50, 0)                             // no name
	r.source("B4", str("Unknown"), str("4790009999990"), 50, 0) // no master product with this barcode
}

func TestConnector_FullRun(t *testing.T) {
	r := newRig(t)
	seedFull(r)
	c := r.create(r.reader)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(false))

	res := r.mustRun(c, enum.RunTriggerManual)
	want := connector.Counts{Read: 8, Changed: 2, Unmatched: 2, Held: 3}
	if res.Status != enum.RunStatusSuccess || res.Counts != want {
		t.Fatalf("run = %+v, want success with %+v", res, want)
	}

	if s := r.sold("A1"); s.ShopPrice != 10500 || s.Managed != "CONNECTOR" {
		t.Errorf("A1 = %+v, want the new price and connector-managed", s)
	}
	// BIZ-15: a zero price, a price over the MRP and a big drop all keep the old price.
	for _, ext := range []string{"A2", "A3", "A4"} {
		if s := r.sold(ext); s.ShopPrice != 10000 {
			t.Errorf("%s price = %d, want the old 10000", ext, s.ShopPrice)
		}
	}
	if n := r.openIssues("PRICE_HOLD"); n != 3 {
		t.Errorf("price holds = %d, want 3", n)
	}
	if s := r.sold("B1"); !s.Found || s.ShopPrice != 9000 || s.Managed != "CONNECTOR" {
		t.Errorf("B1 = %+v, want a new connector-managed product at 9000", s)
	}
	if n := r.openIssues("UNMATCHED"); n != 2 {
		t.Errorf("unmatched = %d, want 2 (B2, B4)", n)
	}
	if r.sold("B3").Found || r.sold("B2").Found {
		t.Error("rows with no name or no match must not be sold")
	}
	if n := r.a.Count(`SELECT count(*) FROM price_changes WHERE source = 'CONNECTOR'`); n != 2 {
		t.Errorf("price changes = %d, want 2 (A1 and B1)", n)
	}
	if n := r.a.Count(`SELECT count(*) FROM products`); n != 5 {
		t.Errorf("products = %d: a run without auto-create must not make master products", n)
	}

	// The same rows again: nothing changes, issues are not doubled (hash skip).
	again := r.mustRun(c, enum.RunTriggerManual)
	if again.Changed != 0 || again.Counts.Held != 0 {
		t.Errorf("second run = %+v, want no work", again)
	}
	if r.openIssues("PRICE_HOLD") != 3 || r.openIssues("UNMATCHED") != 2 {
		t.Error("a repeated run changed the issues")
	}
	if n := r.a.Count(`SELECT count(*) FROM connector_runs WHERE connector_id = $1 AND status = 'SUCCESS'`, c.ID); n != 2 {
		t.Errorf("run rows = %d, want 2", n)
	}

	// A row missing from the source makes the product unavailable, never deleted (FR-SYNC-11).
	r.a.Exec(`DELETE FROM gp_products WHERE external_id = 'A1'`)
	gone := r.mustRun(c, enum.RunTriggerManual)
	if s := r.sold("A1"); !s.Found || s.Available || gone.Changed != 1 {
		t.Errorf("A1 after removal = %+v, run %+v; want unavailable and counted", s, gone)
	}
	r.a.Exec(`INSERT INTO gp_products (external_id, name, price, stock, available) VALUES ('A1', 'Rice', 105, 3, true)`)
	r.mustRun(c, enum.RunTriggerManual)
	if s := r.sold("A1"); !s.Available {
		t.Error("A1 must be available again when it comes back")
	}
}

func TestConnector_EmptySourceHidesNothing(t *testing.T) {
	r := newRig(t)
	seedFull(r)
	c := r.create(r.reader)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(false))
	r.mustRun(c, enum.RunTriggerManual)
	r.a.Exec(`DELETE FROM gp_products`)
	res := r.mustRun(c, enum.RunTriggerManual)
	if res.Changed != 0 || !r.sold("A1").Available {
		t.Errorf("an empty table emptied the shop: %+v", res)
	}
}

func TestConnector_StockAndAvailability(t *testing.T) {
	r := newRig(t)
	seedPreview(r)
	r.a.Exec(`INSERT INTO gp_products (external_id, name, price, stock, available) VALUES
		('A1', 'Rice', 100, 0, true), ('A2', 'Sugar', 100, 5, false), ('A3', 'Tea', 100, 5, true), ('A4', 'Salt', 100, NULL, NULL)`)
	c := r.create(r.reader)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(false))
	r.mustRun(c, enum.RunTriggerManual)
	for ext, avail := range map[string]bool{"A1": false, "A2": false, "A3": true, "A4": true} {
		if got := r.sold(ext).Available; got != avail {
			t.Errorf("%s available = %v, want %v", ext, got, avail)
		}
	}
}

func TestConnector_AutoCreateAndDuplicates(t *testing.T) {
	r := newRig(t)
	r.source("N1", str("New thing"), str("4790005550001"), 60, 0)
	r.source("N2", str("Same barcode"), str("4790005550001"), 61, 0) // same master product as N1
	r.source("N3", str("No barcode"), nil, 70, 0)
	c := r.create(r.reader)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(true))
	res := r.mustRun(c, enum.RunTriggerManual)
	if res.Status != enum.RunStatusSuccess {
		t.Fatalf("run = %+v", res)
	}
	if !r.sold("N1").Found || !r.sold("N3").Found {
		t.Error("auto-create must sell N1 and N3")
	}
	if r.sold("N2").Found {
		t.Error("N2 shares N1's barcode and must not be sold twice")
	}
	if n := r.a.Count(`SELECT count(*) FROM sync_issues WHERE external_id = 'N2' AND kind = 'UNMATCHED' AND status = 'OPEN'`); n != 1 {
		t.Errorf("N2 issues = %d, want 1", n)
	}
	if n := r.a.Count(`SELECT count(*) FROM products WHERE barcode = '4790005550001'`); n != 1 {
		t.Errorf("master products with the barcode = %d, want 1", n)
	}
}
