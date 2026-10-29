package connector_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/connector"
	"gp/internal/connector/source"
	apihttp "gp/internal/http"
	"gp/internal/shared/logx"
	"gp/test/apptest"
)

const sourcePassword = "Zq7-very-unique-password-Zq7"

// rig is the real API plus a "shop database": a table in the test database that
// a login of its own can read, reached through a connection string.
type rig struct {
	a      *apptest.App
	admin  apptest.Client
	shopID uuid.UUID
	reader string // connection string of a read-only login
}

func newRig(t *testing.T) *rig {
	t.Helper()
	a := apptest.New(t)
	r := &rig{a: a, admin: a.AsAdmin(), shopID: a.SeedShop("krishna")}
	a.Exec(`CREATE TABLE gp_products (external_id text PRIMARY KEY, barcode text, name text, brand text, mrp numeric,
	                                   price numeric, stock numeric, available boolean, updated_at timestamptz)`)
	role := "gp_src_" + a.Env.Name[len(a.Env.Name)-8:] // the database name is random, so the role is unique
	for _, q := range []string{
		`CREATE ROLE ` + role + ` LOGIN PASSWORD '` + sourcePassword + `'`,
		`GRANT CONNECT ON DATABASE ` + a.Env.Name + ` TO ` + role,
		`GRANT USAGE ON SCHEMA public TO ` + role,
		`GRANT SELECT ON gp_products TO ` + role,
	} {
		a.Exec(q)
	}
	t.Cleanup(func() {
		a.Exec(`DROP OWNED BY ` + role)
		a.Exec(`DROP ROLE IF EXISTS ` + role)
	})
	u, _ := url.Parse(a.SourceString())
	u.User = url.UserPassword(role, sourcePassword)
	r.reader = u.String()
	return r
}

func (r *rig) path(rest string) string { return "/admin/shops/" + r.shopID.String() + rest }

func (r *rig) create(connString string) connector.Connector {
	r.a.T.Helper()
	body := `{"connectionString":` + strconv.Quote(connString) + `}`
	return apptest.Decode[connector.Connector](r.a.T, r.admin.OK(http.MethodPost, r.path("/connector"), body))
}

func (r *rig) connPath(c connector.Connector, rest string) string {
	return "/admin/connectors/" + c.ID.String() + rest
}

const goodMapping = `{"sourceTable":"gp_products","moneyUnit":"RUPEES","columns":{"externalId":"external_id","name":"name","price":"price",
	"barcode":"barcode","mrp":"mrp","stock":"stock","available":"available","brand":"brand","updatedAt":"updated_at"}}`

func (r *rig) mapped(connString string) connector.Connector {
	r.a.T.Helper()
	c := r.create(connString)
	r.admin.OK(http.MethodPut, r.connPath(c, "/mapping"), goodMapping)
	return c
}

// everything a caller or an operator could see after the calls of a test.
func (r *rig) leaks(t *testing.T, bodies ...string) {
	t.Helper()
	all := strings.Join(bodies, "\n")
	// The masked display string ("postgres://user:•••@host/db") is allowed; the
	// password and any real connection string with a password are not.
	if strings.Contains(all, sourcePassword) {
		t.Errorf("a response contains the password")
	}
	if strings.Contains(all, ":"+sourcePassword+"@") {
		t.Errorf("a response contains a connection string")
	}
	if n := r.a.Count(`SELECT count(*) FROM audit_log WHERE before::text LIKE '%' || $1 || '%' OR after::text LIKE '%' || $1 || '%'`, sourcePassword); n != 0 {
		t.Errorf("the password is in %d audit rows", n)
	}
}

func TestCreate_StoresTheSecretOnlyInTheSecretStore(t *testing.T) {
	r := newRig(t)
	w := r.admin.OK(http.MethodPost, r.path("/connector"), `{"connectionString":`+strconv.Quote(r.reader)+`}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body)
	}
	c := apptest.Decode[connector.Connector](t, w)
	if c.Status != "DRAFT" || c.ShopID != r.shopID || !strings.HasPrefix(c.HostDisplay, "postgres://gp_src_") || !strings.Contains(c.HostDisplay, ":•••@") {
		t.Errorf("connector = %+v", c)
	}

	if got, _ := r.a.Secrets.Get(context.Background(), "connectors/"+c.ID.String()); got != r.reader {
		t.Error("the connection string was not stored under connectors/<id>")
	}
	if n := r.a.Count(`SELECT count(*) FROM connectors WHERE host_display LIKE '%' || $1 || '%' OR secret_ref LIKE '%' || $1 || '%'`, sourcePassword); n != 0 {
		t.Error("the password reached the connectors table")
	}
	get := r.admin.OK(http.MethodGet, r.path("/connector"), "") // DATA-08
	r.leaks(t, w.Body.String(), get.Body.String())
	if n := r.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'connector.create' AND shop_id = $1`, r.shopID); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
}

func TestCreate_Refusals(t *testing.T) {
	r := newRig(t)
	for name, raw := range map[string]string{
		"not a url":     "host=localhost password=" + sourcePassword,
		"no password":   "postgres://reader@db.example.test/shop",
		"host override": "postgres://reader:" + sourcePassword + "@db.example.test/shop?host=10.0.0.1",
		"no encryption": "postgres://reader:" + sourcePassword + "@db.example.test/shop?sslmode=disable",
		"reads a file":  "postgres://reader:" + sourcePassword + "@db.example.test/shop?sslrootcert=/etc/passwd",
		"wrong scheme":  "mysql://reader:" + sourcePassword + "@db.example.test/shop",
	} {
		w := r.admin.Call(http.MethodPost, r.path("/connector"), `{"connectionString":`+strconv.Quote(raw)+`}`)
		e := apptest.ErrorOf(t, w)
		if w.Code != http.StatusBadRequest || e.Error.Code != "VALIDATION_ERROR" || e.Error.Fields["connectionString"] != "invalid" {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
		r.leaks(t, w.Body.String())
	}
	if n := r.a.Count(`SELECT count(*) FROM connectors`); n != 0 {
		t.Errorf("%d connectors were saved from bad strings", n)
	}
	r.admin.Expect(http.MethodPost, "/admin/shops/"+uuid.NewString()+"/connector", `{"connectionString":`+strconv.Quote(r.reader)+`}`, http.StatusNotFound, "NOT_FOUND")

	r.create(r.reader)
	r.admin.Expect(http.MethodPost, r.path("/connector"), `{"connectionString":`+strconv.Quote(r.reader)+`}`, http.StatusConflict, "CONFLICT")
	r.admin.Expect(http.MethodGet, "/admin/shops/"+uuid.NewString()+"/connector", "", http.StatusNotFound, "NOT_FOUND")
}

func TestReplaceSecret_KeepsTheMapping(t *testing.T) {
	r := newRig(t)
	c := r.mapped(r.reader)
	other := "postgres://other_user:" + sourcePassword + "@db2.example.test/shop?sslmode=require"
	w := r.admin.OK(http.MethodPut, r.connPath(c, "/secret"), `{"connectionString":`+strconv.Quote(other)+`}`)
	got := apptest.Decode[connector.Connector](t, w)
	if got.HostDisplay != "postgres://other_user:•••@db2.example.test/shop" || got.Columns["price"] != "price" {
		t.Errorf("connector = %+v", got)
	}
	if v, _ := r.a.Secrets.Get(context.Background(), "connectors/"+c.ID.String()); v != other {
		t.Error("the stored string was not replaced")
	}
	r.leaks(t, w.Body.String())
	r.admin.Expect(http.MethodPut, r.connPath(c, "/secret"), `{"connectionString":"nope"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	r.admin.Expect(http.MethodPut, "/admin/connectors/"+uuid.NewString()+"/secret", `{"connectionString":`+strconv.Quote(other)+`}`, http.StatusNotFound, "NOT_FOUND")
}

func TestTest_ReportsReadOnlyAndWarnsAboutWriteAccess(t *testing.T) {
	r := newRig(t)
	c := r.create(r.reader)
	w := r.admin.OK(http.MethodPost, r.connPath(c, "/test"), "")
	res := apptest.Decode[connector.TestResult](t, w)
	if !res.OK || !res.ReadOnly || res.ServerVersion == "" {
		t.Errorf("read-only login: %+v", res)
	}
	r.leaks(t, w.Body.String())

	// The owner can write anywhere: still ok, with a warning.
	r.admin.OK(http.MethodPut, r.connPath(c, "/secret"), `{"connectionString":`+strconv.Quote(r.a.SourceString())+`}`)
	res = apptest.Decode[connector.TestResult](t, r.admin.OK(http.MethodPost, r.connPath(c, "/test"), ""))
	if !res.OK || res.ReadOnly || !containsWarning(res.Warnings, "read-only user") {
		t.Errorf("owner login: %+v", res)
	}
}

func containsWarning(list []string, part string) bool {
	for _, w := range list {
		if strings.Contains(w, part) {
			return true
		}
	}
	return false
}

func TestTest_FailureIsOneFixedMessageAndTheLogHoldsNoSecrets(t *testing.T) {
	r := newRig(t)
	var logs bytes.Buffer
	logx.SetDefault(logx.New(slog.LevelDebug, &logs))
	t.Cleanup(func() { logx.SetDefault(logx.New(slog.LevelInfo, os.Stdout)) })

	u, _ := url.Parse(r.reader)
	u.User = url.UserPassword(u.User.Username(), "not-the-password")
	c := r.create(u.String())
	w := r.admin.Call(http.MethodPost, r.connPath(c, "/test"), "")
	e := apptest.ErrorOf(t, w)
	if w.Code != http.StatusUnprocessableEntity || e.Error.Code != "CONNECTOR_TEST_FAILED" {
		t.Fatalf("test = %d %s", w.Code, w.Body)
	}
	for _, leak := range []string{"not-the-password", u.User.Username(), u.Hostname(), "authentication", "password"} {
		if strings.Contains(w.Body.String(), leak) || strings.Contains(logs.String(), leak) {
			t.Errorf("%q appears in the response or the log", leak)
		}
	}
	if !strings.Contains(logs.String(), "connector.source_failed") || !strings.Contains(logs.String(), "28P01") {
		t.Errorf("the log should say where it failed: %s", logs.String())
	}
}

type resolver map[string]string

func (m resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if ip, ok := m[host]; ok {
		return []netip.Addr{netip.MustParseAddr(ip)}, nil
	}
	return nil, os.ErrNotExist
}

// SSRF-01 and SSRF-02 through the API, with the production policy: every inside
// target and every other port gives the same answer, and nothing is connected.
func TestTest_SSRF(t *testing.T) {
	r := newRig(t)
	prod := connector.NewService(r.a.Env.DB, r.a.Secrets,
		source.NewPostgres(source.Options{Policy: source.Policy{Resolver: resolver{"rebind.example.test": "127.0.0.1"}}}), apptest.ImgBase)
	deps := r.a.Deps
	deps.Connector = connector.NewHandler(prod)
	r.a.Handler = apihttp.New(deps)

	c := r.create(r.reader)
	var bodies = map[string]bool{}
	for _, host := range []string{
		"localhost", "127.0.0.1", "[::1]", "169.254.169.254", "10.0.0.5", "172.16.1.1", "192.168.0.10", "100.64.0.1",
		"2130706433", "0x7f000001", "127.1", "rebind.example.test", "[::ffff:127.0.0.1]", "[fd00:ec2::254]",
		"8.8.8.8:22", "8.8.8.8:80", "8.8.8.8:5433", "db.example.test:5432", // the last cannot resolve at all
	} {
		raw := "postgres://reader:" + sourcePassword + "@" + host + "/shop?sslmode=require"
		r.admin.OK(http.MethodPut, r.connPath(c, "/secret"), `{"connectionString":`+strconv.Quote(raw)+`}`)
		w := r.admin.Call(http.MethodPost, r.connPath(c, "/test"), "")
		e := apptest.ErrorOf(t, w)
		if w.Code != http.StatusUnprocessableEntity || e.Error.Code != "CONNECTOR_TEST_FAILED" {
			t.Errorf("%s: %d %s", host, w.Code, w.Body)
		}
		bodies[strings.Join([]string{e.Error.Code, strconv.Itoa(w.Code)}, "|")] = true
		if strings.Contains(w.Body.String(), host) {
			t.Errorf("%s: the response names the target", host)
		}
		// Tables, columns and preview go through the same door.
		for _, call := range [][2]string{{http.MethodGet, "/tables"}, {http.MethodGet, "/columns?table=gp_products"}, {http.MethodPost, "/preview"}} {
			if w := r.admin.Call(call[0], r.connPath(c, call[1]), ""); w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusConflict {
				t.Errorf("%s %s%s = %d", host, call[0], call[1], w.Code)
			}
		}
	}
	if len(bodies) != 1 {
		t.Errorf("the answers differ: %v", bodies)
	}
}

func TestTablesAndColumns(t *testing.T) {
	r := newRig(t)
	c := r.create(r.reader)
	type table struct{ Schema, Name, Kind string }
	tables := apptest.Decode[struct{ Data []table }](t, r.admin.OK(http.MethodGet, r.connPath(c, "/tables"), "")).Data
	if len(tables) != 1 || tables[0] != (table{"public", "gp_products", "TABLE"}) {
		t.Errorf("tables = %v, want only what the login may read", tables)
	}
	type column struct{ Name, Type string }
	cols := apptest.Decode[struct{ Data []column }](t, r.admin.OK(http.MethodGet, r.connPath(c, "/columns?table=gp_products"), "")).Data
	if len(cols) != 9 || cols[0] != (column{"external_id", "text"}) || cols[5] != (column{"price", "numeric"}) {
		t.Errorf("columns = %v", cols)
	}
	r.admin.Expect(http.MethodGet, r.connPath(c, "/columns?table=missing"), "", http.StatusNotFound, "NOT_FOUND")
	r.admin.Expect(http.MethodGet, r.connPath(c, "/columns?table=pg_authid"), "", http.StatusNotFound, "NOT_FOUND")
	for _, bad := range []string{"", "Gp_Products", url.QueryEscape(`x"; DROP TABLE gp_products;--`), "a.b.c"} {
		r.admin.Expect(http.MethodGet, r.connPath(c, "/columns?table="+bad), "", http.StatusBadRequest, "VALIDATION_ERROR")
	}
	r.admin.Expect(http.MethodGet, "/admin/connectors/"+uuid.NewString()+"/tables", "", http.StatusNotFound, "NOT_FOUND")
}

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
