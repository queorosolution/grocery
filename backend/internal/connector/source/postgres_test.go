package source_test

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"gp/internal/connector/source"
	"gp/test/testdb"
)

// rig is a real Postgres with a small shop table, reachable the way a shop's
// database would be (a connection string), minus TLS.
type rig struct {
	env  *testdb.Env
	open *source.Postgres
	str  string // owner connection string
	url  *url.URL
}

func newRig(t *testing.T) *rig {
	t.Helper()
	env := testdb.New(t)
	u, err := url.Parse(env.OwnerURL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	q := url.Values{"sslmode": {"require"}}
	u.RawQuery = q.Encode()
	r := &rig{
		env: env, url: u, str: u.String(),
		open: source.NewPostgres(source.Options{Policy: source.Policy{AllowLocal: true, Port: port}, PlaintextForTest: true}),
	}
	for _, q := range []string{
		`CREATE TABLE gp_items (id int PRIMARY KEY, name text, price numeric(12,2), stock int, available boolean, updated_at timestamptz)`,
		`INSERT INTO gp_items VALUES (3, 'Dhal 1kg', 480.50, NULL, true, '2026-10-01 10:00+00'),
		                             (1, 'Rice 5kg', 1250, 12, false, '2026-10-02 10:00+00'),
		                             (2, NULL, 0, 0, NULL, NULL)`,
		`CREATE VIEW gp_products AS SELECT id AS external_id, name, price FROM gp_items`,
		`CREATE TABLE "Mixed" (id int)`,
	} {
		if _, err := env.Owner.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// readOnlyString creates a login that can only read gp_products and returns its connection string.
func (r *rig) readOnlyString(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	role := "gp_ro_" + r.env.Name[len(r.env.Name)-8:] // the database name is random, so the role is unique too
	for _, q := range []string{
		`CREATE ROLE ` + role + ` LOGIN PASSWORD 'ro-pass'`,
		`GRANT CONNECT ON DATABASE ` + r.env.Name + ` TO ` + role,
		`GRANT USAGE ON SCHEMA public TO ` + role,
		`GRANT SELECT ON gp_products TO ` + role,
	} {
		if _, err := r.env.Owner.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = r.env.Owner.Exec(ctx, `DROP OWNED BY `+role)
		_, _ = r.env.Owner.Exec(ctx, `DROP ROLE IF EXISTS `+role)
	})
	u := *r.url
	u.User = url.UserPassword(role, "ro-pass")
	return u.String()
}

func (r *rig) conn(t *testing.T, connString string) source.Conn {
	t.Helper()
	c, err := r.open.Open(context.Background(), connString)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestTest_ReportsVersionAndWriteRights(t *testing.T) {
	r := newRig(t)
	owner, err := r.conn(t, r.str).Test(context.Background())
	if err != nil || owner.ServerVersion == "" || !owner.CanWrite || owner.TLS {
		t.Errorf("owner = %+v, %v; want a version, CanWrite and no TLS (the test server has none)", owner, err)
	}
	reader, err := r.conn(t, r.readOnlyString(t)).Test(context.Background())
	if err != nil || reader.CanWrite {
		t.Errorf("read-only login = %+v, %v; want CanWrite false", reader, err)
	}
}

func TestTables_ListsReadableUsableNamesOnly(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	got, err := r.conn(t, r.str).Tables(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, tb := range got {
		kinds[tb.String()] = tb.Kind
	}
	if kinds["public.gp_items"] != "TABLE" || kinds["public.gp_products"] != "VIEW" {
		t.Errorf("tables = %v", kinds)
	}
	if _, ok := kinds["public.Mixed"]; ok {
		t.Error(`a table named "Mixed" cannot be mapped but was listed`)
	}
	for name := range kinds {
		if strings.HasPrefix(name, "pg_") || strings.HasPrefix(name, "information_schema") {
			t.Errorf("system table %s listed", name)
		}
	}

	ro, err := r.conn(t, r.readOnlyString(t)).Tables(ctx)
	if err != nil || len(ro) != 1 || ro[0].String() != "public.gp_products" {
		t.Errorf("read-only login sees %v, %v; want only the granted view", ro, err)
	}
}

func TestColumns(t *testing.T) {
	r := newRig(t)
	cols, err := r.conn(t, r.str).Columns(context.Background(), source.TableName{Schema: "public", Name: "gp_items"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range cols {
		names = append(names, c.Name+":"+c.Type)
	}
	want := "id:integer name:text price:numeric stock:integer available:boolean updated_at:timestamp with time zone"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("columns = %s\nwant      %s", got, want)
	}
	if none, err := r.conn(t, r.str).Columns(context.Background(), source.TableName{Schema: "public", Name: "nothing"}); err != nil || len(none) != 0 {
		t.Errorf("unknown table columns = %v, %v", none, err)
	}
}

func TestRead_OrdersLimitsAndKeepsNulls(t *testing.T) {
	r := newRig(t)
	var rows []source.Row
	err := r.conn(t, r.str).Read(context.Background(), source.ReadSpec{
		Table: source.TableName{Schema: "public", Name: "gp_items"}, Columns: []string{"id", "name", "price", "available"}, OrderBy: "id", Limit: 2,
	}, func(row source.Row) error { rows = append(rows, row); return nil })
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %d, %v", len(rows), err)
	}
	if *rows[0][0] != "1" || *rows[0][1] != "Rice 5kg" || *rows[0][2] != "1250.00" || *rows[0][3] != "false" {
		t.Errorf("first row = %q %q %q %q", *rows[0][0], *rows[0][1], *rows[0][2], *rows[0][3])
	}
	if *rows[1][0] != "2" || rows[1][1] != nil || *rows[1][2] != "0.00" || rows[1][3] != nil {
		t.Errorf("second row should keep its NULLs: %v", rows[1])
	}
}

// SQLI-02: a name that is not a column of the table never reaches the SQL.
func TestRead_RefusesInjectedNames(t *testing.T) {
	r := newRig(t)
	c := r.conn(t, r.str)
	tbl := source.TableName{Schema: "public", Name: "gp_items"}
	bad := []string{`price"; DROP TABLE gp_items;--`, `id, (SELECT password FROM pg_authid)`, "nope", `"id"`, "ID", ""}
	for _, name := range bad {
		for _, spec := range []source.ReadSpec{
			{Table: tbl, Columns: []string{name}, OrderBy: "id", Limit: 1},
			{Table: tbl, Columns: []string{"id"}, OrderBy: name, Limit: 1},
		} {
			if err := c.Read(context.Background(), spec, func(source.Row) error { return nil }); err == nil {
				t.Errorf("name %q accepted: %+v", name, spec)
			}
		}
	}
	for _, tb := range []source.TableName{{Schema: "public", Name: `gp_items"; DROP TABLE gp_items;--`}, {Schema: "pg_catalog", Name: "pg_authid"}, {Schema: "public", Name: "missing"}} {
		spec := source.ReadSpec{Table: tb, Columns: []string{"id"}, OrderBy: "id", Limit: 1}
		if err := c.Read(context.Background(), spec, func(source.Row) error { return nil }); err == nil {
			t.Errorf("table %v accepted", tb)
		}
	}
	var n int
	if err := r.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM gp_items`).Scan(&n); err != nil || n != 3 {
		t.Errorf("gp_items rows = %d, %v; the table must be untouched", n, err)
	}
}

func TestOpen_ProductionPolicyRefusesLocalTargets(t *testing.T) {
	prod := source.NewPostgres(source.Options{})
	for _, raw := range []string{
		"postgres://u:topsecret@127.0.0.1:5432/d?sslmode=require",
		"postgres://u:topsecret@localhost/d?sslmode=require",
		"postgres://u:topsecret@169.254.169.254/d?sslmode=require",
		"postgres://u:topsecret@[::1]/d?sslmode=require",
		"postgres://u:topsecret@8.8.8.8:22/d?sslmode=require", // SSRF-02: port 22
		"postgres://u:topsecret@2130706433/d?sslmode=require",
	} {
		_, err := prod.Open(context.Background(), raw)
		var oe *source.OpenError
		if !errors.As(err, &oe) || oe.Stage != "guard" {
			t.Errorf("%s: err = %v, want a guard refusal", raw, err)
			continue
		}
		if strings.Contains(err.Error(), "topsecret") || strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("error leaks details: %v", err)
		}
	}
}

func TestOpen_FailuresCarryNoSecrets(t *testing.T) {
	r := newRig(t)
	u := *r.url
	u.User = url.UserPassword("postgres", "definitely-wrong-password")
	_, err := r.open.Open(context.Background(), u.String())
	var oe *source.OpenError
	if !errors.As(err, &oe) || oe.Stage != "connect" || oe.Code != "28P01" {
		t.Fatalf("err = %v, want a connect failure with SQLSTATE 28P01", err)
	}
	if strings.Contains(err.Error(), "wrong-password") || strings.Contains(err.Error(), "postgres") {
		t.Errorf("error leaks details: %v", err)
	}
	if _, err := r.open.Open(context.Background(), "postgres://broken"); err == nil {
		t.Error("a bad string was accepted")
	}
}
