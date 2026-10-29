package connector_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/connector"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

type issueList struct {
	Data []connector.Issue `json:"data"`
}

func (r *rig) issues(query string) []connector.Issue {
	r.a.T.Helper()
	return apptest.Decode[issueList](r.a.T, r.admin.OK(http.MethodGet, "/admin/sync-issues"+query, "")).Data
}

func (r *rig) issueFor(ext, kind string) connector.Issue {
	r.a.T.Helper()
	for _, i := range r.issues("?status=OPEN") {
		if i.ExternalID == ext && string(i.Kind) == kind {
			return i
		}
	}
	r.a.T.Fatalf("no open %s issue for %s", kind, ext)
	return connector.Issue{}
}

func (r *rig) master(name string, barcode *string) uuid.UUID {
	var id uuid.UUID
	if err := r.a.Env.Owner.QueryRow(context.Background(), `INSERT INTO products (name, barcode) VALUES ($1, $2) RETURNING id`, name, barcode).Scan(&id); err != nil {
		r.a.T.Fatal(err)
	}
	return id
}

// issueRig runs one sync that leaves four price holds and two unmatched rows.
func issueRig(t *testing.T) (*rig, connector.Connector) {
	r := newRig(t)
	seedFull(r)
	r.master("Jam", str("4790001112223"))
	r.source("C1", str("Jam"), str("4790001112223"), 200, 150) // a new product whose price is above its MRP
	c := r.create(r.reader)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(false))
	r.mustRun(c, enum.RunTriggerManual)
	return r, c
}

func TestConnector_IssueList(t *testing.T) {
	r, c := issueRig(t)
	all := r.issues("")
	if len(all) != 6 || r.openIssues("PRICE_HOLD") != 4 || r.openIssues("UNMATCHED") != 2 {
		t.Fatalf("issues = %d, holds %d, unmatched %d; want 6, 4, 2", len(all), r.openIssues("PRICE_HOLD"), r.openIssues("UNMATCHED"))
	}
	if got := r.issues("?status=RESOLVED"); len(got) != 0 {
		t.Errorf("resolved issues = %d, want none", len(got))
	}
	if got := r.issues("?shopId=" + r.shopID.String()); len(got) != 6 {
		t.Errorf("issues of the shop = %d", len(got))
	}
	if got := r.issues("?shopId=" + uuid.NewString()); len(got) != 0 {
		t.Errorf("issues of another shop = %d", len(got))
	}
	r.admin.Expect(http.MethodGet, "/admin/sync-issues?status=DONE", "", http.StatusBadRequest, "VALIDATION_ERROR")
	r.admin.Expect(http.MethodGet, "/admin/sync-issues?shopId=x", "", http.StatusBadRequest, "VALIDATION_ERROR")

	i := r.issueFor("A3", "PRICE_HOLD")
	p := i.Payload
	if i.ConnectorID != c.ID || p.Reason != connector.ReasonOverMRP || p.PriceCents == nil || *p.PriceCents != 13000 ||
		p.CurrentPriceCents == nil || *p.CurrentPriceCents != 10000 || p.ShopProductID == nil {
		t.Errorf("A3 issue = %+v", i)
	}
	// An issue keeps the row, and nothing about the connection.
	r.leaks(t, r.admin.OK(http.MethodGet, "/admin/sync-issues", "").Body.String())

	// A hold clears itself when the shop fixes the price.
	r.a.Exec(`UPDATE gp_products SET price = 95 WHERE external_id = 'A2'`)
	r.mustRun(c, enum.RunTriggerManual)
	if r.sold("A2").ShopPrice != 9500 || r.openIssues("PRICE_HOLD") != 3 {
		t.Errorf("A2 = %+v, holds %d; want 9500 and the hold closed", r.sold("A2"), r.openIssues("PRICE_HOLD"))
	}
	if n := r.a.Count(`SELECT count(*) FROM sync_issues WHERE external_id = 'A2' AND status = 'RESOLVED' AND resolved_by IS NULL`); n != 1 {
		t.Error("the sync should close the A2 issue by itself")
	}
}

func TestConnector_ResolveUnmatched(t *testing.T) {
	r, _ := issueRig(t)
	loose := r.master("Loose item", nil)
	b2, b4 := r.issueFor("B2", "UNMATCHED"), r.issueFor("B4", "UNMATCHED")

	r.admin.Expect(http.MethodPost, "/admin/sync-issues/"+b2.ID.String()+"/resolve", `{"action":"LINK"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	r.admin.Expect(http.MethodPost, "/admin/sync-issues/"+b2.ID.String()+"/resolve", `{"action":"LINK","productId":"`+uuid.NewString()+`"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	r.admin.Expect(http.MethodPost, "/admin/sync-issues/"+b2.ID.String()+"/resolve", `{"action":"APPROVE_PRICE"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	r.admin.Expect(http.MethodPost, "/admin/sync-issues/"+b2.ID.String()+"/resolve", `{"action":"NOPE"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	if r.sold("B2").Found {
		t.Fatal("a refused action sold the product")
	}

	got := apptest.Decode[connector.Issue](t, r.admin.OK(http.MethodPost, "/admin/sync-issues/"+b2.ID.String()+"/resolve", `{"action":"LINK","productId":"`+loose.String()+`"}`))
	if got.Status != enum.SyncIssueStatusResolved || r.sold("B2").ShopPrice != 5000 || r.sold("B2").Managed != "CONNECTOR" {
		t.Errorf("after LINK: %+v, B2 = %+v", got, r.sold("B2"))
	}
	if n := r.a.Count(`SELECT count(*) FROM sync_issues WHERE id = $1 AND resolved_by IS NOT NULL AND resolved_at IS NOT NULL`, b2.ID); n != 1 {
		t.Error("the admin who resolved it is not recorded")
	}
	// The same product cannot be sold twice, and a closed issue cannot be acted on again.
	r.admin.Expect(http.MethodPost, "/admin/sync-issues/"+b2.ID.String()+"/resolve", `{"action":"IGNORE"}`, http.StatusConflict, "CONFLICT")

	r.admin.OK(http.MethodPost, "/admin/sync-issues/"+b4.ID.String()+"/resolve", `{"action":"CREATE"}`)
	if n := r.a.Count(`SELECT count(*) FROM products WHERE barcode = '4790009999990' AND name = 'Unknown'`); n != 1 || !r.sold("B4").Found {
		t.Error("CREATE should make the master product and sell it")
	}
	if n := r.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'sync_issue.resolve'`); n != 2 {
		t.Errorf("audit rows = %d, want 2", n)
	}
	r.admin.Expect(http.MethodPost, "/admin/sync-issues/"+uuid.NewString()+"/resolve", `{"action":"IGNORE"}`, http.StatusNotFound, "NOT_FOUND")
}

func TestConnector_ResolvePriceHolds(t *testing.T) {
	r, _ := issueRig(t)
	path := func(i connector.Issue) string { return "/admin/sync-issues/" + i.ID.String() + "/resolve" }

	// A drop of more than half, once approved, is taken as it is.
	r.admin.OK(http.MethodPost, path(r.issueFor("A2", "PRICE_HOLD")), `{"action":"APPROVE_PRICE"}`)
	if r.sold("A2").ShopPrice != 4000 {
		t.Errorf("A2 = %+v, want 4000", r.sold("A2"))
	}
	// A price above the MRP is taken, and the out-of-date MRP is dropped.
	r.admin.OK(http.MethodPost, path(r.issueFor("A3", "PRICE_HOLD")), `{"action":"APPROVE_PRICE"}`)
	var mrp *int64
	if err := r.a.Env.Owner.QueryRow(context.Background(), `SELECT mrp_cents FROM shop_products WHERE external_id = 'A3' AND shop_id = $1`, r.shopID).Scan(&mrp); err != nil {
		t.Fatal(err)
	}
	if r.sold("A3").ShopPrice != 13000 || mrp != nil {
		t.Errorf("A3 = %+v, mrp %v; want 13000 and no MRP", r.sold("A3"), mrp)
	}
	// A hold on a product the shop does not sell yet makes it.
	r.admin.OK(http.MethodPost, path(r.issueFor("C1", "PRICE_HOLD")), `{"action":"APPROVE_PRICE"}`)
	if s := r.sold("C1"); !s.Found || s.ShopPrice != 20000 || s.Managed != "CONNECTOR" {
		t.Errorf("C1 = %+v, want sold at 20000", s)
	}
	if n := r.a.Count(`SELECT count(*) FROM products WHERE name = 'Jam'`); n != 1 {
		t.Errorf("Jam products = %d: approving must use the existing master product", n)
	}
	// A price of 0 is never approved, only rejected.
	a4 := r.issueFor("A4", "PRICE_HOLD")
	r.admin.Expect(http.MethodPost, path(a4), `{"action":"APPROVE_PRICE"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	r.admin.OK(http.MethodPost, path(a4), `{"action":"REJECT_PRICE"}`)
	if r.sold("A4").ShopPrice != 10000 || r.openIssues("PRICE_HOLD") != 0 {
		t.Errorf("A4 = %+v, holds %d; want the old price and no open holds", r.sold("A4"), r.openIssues("PRICE_HOLD"))
	}
	if n := r.a.Count(`SELECT count(*) FROM price_changes WHERE source = 'CONNECTOR'`); n < 4 {
		t.Errorf("price changes = %d", n)
	}
}

func TestConnector_IgnoreStaysIgnoredUntilTheRowChanges(t *testing.T) {
	r := newRig(t)
	r.source("Z1", str("Odd item"), nil, 50, 0)
	c := r.create(r.reader)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(false))
	r.mustRun(c, enum.RunTriggerManual)
	z := r.issueFor("Z1", "UNMATCHED")
	r.admin.OK(http.MethodPost, "/admin/sync-issues/"+z.ID.String()+"/resolve", `{"action":"IGNORE"}`)

	r.mustRun(c, enum.RunTriggerManual) // the same row is read again
	if r.openIssues("UNMATCHED") != 0 || len(r.issues("?status=IGNORED")) != 1 {
		t.Fatalf("an ignored row came back: open %d, ignored %d", r.openIssues("UNMATCHED"), len(r.issues("?status=IGNORED")))
	}
	r.a.Exec(`UPDATE gp_products SET name = 'Odd item, renamed' WHERE external_id = 'Z1'`)
	r.mustRun(c, enum.RunTriggerManual)
	if r.openIssues("UNMATCHED") != 1 {
		t.Error("a changed row should be reported again")
	}
}
