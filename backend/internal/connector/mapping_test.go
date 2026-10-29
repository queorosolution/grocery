package connector_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/connector"
	"gp/test/apptest"
)

func TestMapping_Saves(t *testing.T) {
	r := newRig(t)
	c := r.create(r.reader)
	got := apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), goodMapping))
	if *got.SourceTable != "public.gp_products" || got.MoneyUnit != "RUPEES" || got.IntervalMin != 60 || got.AutoCreateProducts ||
		got.Columns["price"] != "price" || got.Columns["externalId"] != "external_id" {
		t.Errorf("mapping = %+v", got)
	}
	if got.ChangeColumn == nil || *got.ChangeColumn != "updated_at" {
		t.Errorf("changeColumn = %v, want updated_at taken from the updatedAt column", got.ChangeColumn)
	}

	// The whole mapping is replaced, and the cursor starts over.
	r.a.Exec(`UPDATE connectors SET last_cursor = now() WHERE id = $1`, c.ID)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"),
		`{"sourceTable":"public.gp_products","moneyUnit":"CENTS","changeColumn":"updated_at","intervalMin":15,"autoCreateProducts":true,"columns":{"externalId":"external_id","name":"name","price":"price"}}`)
	got = apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodGet, r.path("/connector"), ""))
	if got.MoneyUnit != "CENTS" || got.IntervalMin != 15 || !got.AutoCreateProducts || len(got.Columns) != 3 {
		t.Errorf("replaced mapping = %+v", got)
	}
	if n := r.a.Count(`SELECT count(*) FROM connectors WHERE id = $1 AND last_cursor IS NULL`, c.ID); n != 1 {
		t.Error("the cursor was not reset")
	}
	if n := r.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'connector.mapping.set'`); n != 2 {
		t.Errorf("audit rows = %d, want 2", n)
	}
}

// SQLI-02: names that are not real columns never reach the SQL.
func TestMapping_Refusals(t *testing.T) {
	r := newRig(t)
	c := r.create(r.reader)
	cols := func(extra string) string {
		return `"columns":{"externalId":"external_id","name":"name","price":"price"` + extra + `}`
	}
	tests := map[string]struct{ body, field string }{
		"injected column":     {`{"sourceTable":"gp_products",` + cols(`,"brand":"price\"; DROP TABLE gp_products;--"`) + `}`, "columns.brand"},
		"injected table":      {`{"sourceTable":"gp_products\"; DROP TABLE gp_products;--",` + cols("") + `}`, "sourceTable"},
		"system table":        {`{"sourceTable":"pg_catalog.pg_authid",` + cols("") + `}`, "sourceTable"},
		"missing table":       {`{"sourceTable":"nothing",` + cols("") + `}`, "sourceTable"},
		"column not in table": {`{"sourceTable":"gp_products",` + cols(`,"brand":"no_such_column"`) + `}`, "columns.brand"},
		"upper case column":   {`{"sourceTable":"gp_products",` + cols(`,"brand":"Brand"`) + `}`, "columns.brand"},
		"required missing":    {`{"sourceTable":"gp_products","columns":{"externalId":"external_id","name":"name"}}`, "columns.price"},
		"unknown field":       {`{"sourceTable":"gp_products",` + cols(`,"secret":"name"`) + `}`, "columns.secret"},
		"bad money unit":      {`{"sourceTable":"gp_products","moneyUnit":"EUROS",` + cols("") + `}`, "moneyUnit"},
		"interval too short":  {`{"sourceTable":"gp_products","intervalMin":5,` + cols("") + `}`, "intervalMin"},
		"interval too long":   {`{"sourceTable":"gp_products","intervalMin":2000,` + cols("") + `}`, "intervalMin"},
		"change not a time":   {`{"sourceTable":"gp_products","changeColumn":"price",` + cols("") + `}`, "changeColumn"},
		"change not a column": {`{"sourceTable":"gp_products","changeColumn":"nope",` + cols("") + `}`, "changeColumn"},
		"change differs":      {`{"sourceTable":"gp_products","changeColumn":"updated_at",` + cols(`,"updatedAt":"price"`) + `}`, "changeColumn"},
	}
	for name, tc := range tests {
		w := r.admin.Call(http.MethodPut, r.connPath(c, "/mapping"), tc.body)
		e := apptest.ErrorOf(t, w)
		if w.Code != http.StatusBadRequest || e.Error.Code != "VALIDATION_ERROR" || e.Error.Fields[tc.field] == "" {
			t.Errorf("%s: %d %s (want a flag on %s)", name, w.Code, w.Body, tc.field)
		}
	}
	if n := r.a.Count(`SELECT count(*) FROM connectors WHERE source_table IS NOT NULL`); n != 0 {
		t.Error("a refused mapping was saved")
	}
	if n := r.a.Count(`SELECT count(*) FROM gp_products`); n != 0 {
		t.Error("the source table changed")
	}
	if n := r.a.Count(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'gp_products'`); n != 1 {
		t.Error("the source table is gone")
	}
	r.admin.Expect(http.MethodPut, "/admin/connectors/"+uuid.NewString()+"/mapping", goodMapping, http.StatusNotFound, "NOT_FOUND")
}
