package connector_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gp/internal/connector"
	"gp/test/apptest"
)

const barcodeKnown = "4790001234567"

// seedPreview puts the shop's side in place: three shop products already linked
// to source ids, and a master product with a barcode the shop does not sell yet.
func seedPreview(r *rig) {
	for _, p := range []struct{ ext, name string }{{"A1", "Rice"}, {"A2", "Sugar"}, {"A3", "Tea"}, {"A4", "Salt"}} {
		r.a.Exec(`WITH p AS (INSERT INTO products (name) VALUES ($1) RETURNING id)
			INSERT INTO shop_products (shop_id, product_id, external_id, shop_price_cents, online_price_cents, mrp_cents)
			SELECT $2, p.id, $3, 10000, 10000, 12000 FROM p`, p.name, r.shopID, p.ext)
	}
	r.a.Exec(`INSERT INTO products (name, barcode) VALUES ('Biscuits', $1)`, barcodeKnown)
}

func (r *rig) source(ext string, name *string, barcode *string, price, mrp float64) {
	r.a.Exec(`INSERT INTO gp_products (external_id, name, barcode, price, mrp, stock, available) VALUES ($1, $2, $3, $4, $5, 3, true)`,
		ext, name, barcode, price, mrp)
}

func str(s string) *string { return &s }

func mappingWith(autoCreate bool) string {
	return fmt.Sprintf(`{"sourceTable":"gp_products","moneyUnit":"RUPEES","autoCreateProducts":%t,"columns":{"externalId":"external_id",
		"name":"name","price":"price","barcode":"barcode","mrp":"mrp","stock":"stock","available":"available"}}`, autoCreate)
}

type previewResponse struct {
	Rows []connector.PreviewRow `json:"rows"`
}

func previewBody(t *testing.T, w *httptest.ResponseRecorder) previewResponse {
	t.Helper()
	return apptest.Decode[previewResponse](t, w)
}

func (r *rig) preview(c connector.Connector) map[string]connector.PreviewRow {
	r.a.T.Helper()
	w := r.admin.OK(http.MethodPost, r.connPath(c, "/preview"), "")
	rows := previewBody(r.a.T, w).Rows
	out := map[string]connector.PreviewRow{}
	for _, row := range rows {
		out[row.ExternalID] = row
	}
	return out
}

func TestPreview_SaysWhatASyncWouldDoAndWritesNothing(t *testing.T) {
	r := newRig(t)
	seedPreview(r)
	r.source("A1", str("Rice"), nil, 105, 0)                    // update
	r.source("A2", str("Sugar"), nil, 40, 0)                    // more than 50% down
	r.source("A3", str("Tea"), nil, 130, 120)                   // above the MRP
	r.source("A4", str("Salt"), nil, 0, 0)                      // zero price
	r.source("B1", str("Biscuits"), str(barcodeKnown), 90, 100) // new link by barcode
	r.source("B2", str("Loose item"), nil, 50, 0)               // nothing to match by
	r.source("B3", nil, nil, 50, 0)                             // no name
	r.source("B4", str("Unknown"), str("4790009999990"), 50, 0)

	c := r.create(r.reader)
	r.admin.Expect(http.MethodPost, r.connPath(c, "/preview"), "", http.StatusConflict, "CONFLICT") // no mapping yet
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(false))

	shopBefore := r.a.Count(`SELECT count(*) FROM shop_products`)
	productsBefore := r.a.Count(`SELECT count(*) FROM products`)
	got := r.preview(c)

	want := map[string][2]string{
		"A1": {"update", ""},
		"A2": {"hold", "big_drop"},
		"A3": {"hold", "over_mrp"},
		"A4": {"hold", "zero_price"},
		"B1": {"create", "barcode"},
		"B2": {"unmatched", "no_barcode"},
		"B3": {"invalid", ""},
		"B4": {"unmatched", "unknown_barcode"},
	}
	for ext, w := range want {
		row, ok := got[ext]
		if !ok || string(row.Action) != w[0] || (w[1] != "" && row.Reason != w[1]) {
			t.Errorf("%s = %+v, want %v", ext, row, w)
		}
	}
	if a1 := got["A1"]; a1.PriceCents == nil || *a1.PriceCents != 10500 {
		t.Errorf("rupees were not turned into cents: %+v", a1.PriceCents)
	}

	// With auto-create on, the rows with no match become new products.
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), mappingWith(true))
	got = r.preview(c)
	if row := got["B2"]; row.Action != connector.ActionCreate || row.Reason != connector.ReasonAutoCreate {
		t.Errorf("B2 with auto-create = %+v", row)
	}
	if row := got["B3"]; row.Action != connector.ActionInvalid {
		t.Errorf("an unnamed row must stay invalid: %+v", row)
	}

	if n := r.a.Count(`SELECT count(*) FROM shop_products`); n != shopBefore {
		t.Error("preview wrote shop products")
	}
	if n := r.a.Count(`SELECT count(*) FROM products`); n != productsBefore {
		t.Error("preview wrote products")
	}
	if n := r.a.Count(`SELECT count(*) FROM gp_products`); n != 8 {
		t.Error("preview changed the source table")
	}
}

func TestPreview_CentsAndTheTwentyRowLimit(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 30; i++ {
		r.source(fmt.Sprintf("X%02d", i), str(fmt.Sprintf("Item %02d", i)), nil, 250, 0)
	}
	c := r.create(r.reader)
	body := `{"sourceTable":"gp_products","moneyUnit":"CENTS","columns":{"externalId":"external_id","name":"name","price":"price"}}`
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), body)

	w := r.admin.OK(http.MethodPost, r.connPath(c, "/preview"), "")
	rows := previewBody(t, w).Rows
	if len(rows) != 20 || rows[0].ExternalID != "X00" || rows[19].ExternalID != "X19" {
		t.Fatalf("rows = %d, first %q, last %q", len(rows), rows[0].ExternalID, rows[len(rows)-1].ExternalID)
	}
	if rows[0].PriceCents == nil || *rows[0].PriceCents != 250 {
		t.Errorf("cents were changed: %v", rows[0].PriceCents)
	}
	r.admin.Expect(http.MethodPost, "/admin/connectors/00000000-0000-4000-8000-000000000000/preview", "", http.StatusNotFound, "NOT_FOUND")
}
