package audit_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/test/testdb"
)

func write(t *testing.T, env *testdb.Env, scope db.Scope, e audit.Entry) {
	t.Helper()
	err := env.DB.Tx(context.Background(), scope, func(ctx context.Context, tx *db.Tx) error {
		return audit.Write(ctx, tx, scope, e)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func list(t *testing.T, env *testdb.Env, q audit.Query) audit.Page {
	t.Helper()
	var page audit.Page
	err := env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		page, err = audit.List(ctx, tx, q)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestWriteKeepsFactsAndMasksSecrets(t *testing.T) {
	env := testdb.New(t)
	admin := uuid.New()
	env.Owner.Exec(context.Background(), `INSERT INTO users (id, google_sub, email, name) VALUES ($1, 'a', 'a@example.test', 'A')`, admin)
	write(t, env, db.Scope{UserID: admin, Privileged: true, Role: enum.RoleSuperAdmin}, audit.Entry{
		Action: "payment_account.update", Entity: "payment_account", EntityID: "acc-1",
		Before: map[string]any{"merchantId": "12345", "phone": "+94771234567"},
		After:  map[string]any{"merchantSecret": "hunter2", "merchantId": "99999"},
	})

	page := list(t, env, audit.Query{})
	if len(page.Data) != 1 {
		t.Fatalf("rows = %d", len(page.Data))
	}
	r := page.Data[0]
	if r.ActorID == nil || *r.ActorID != admin || r.ActorRole != "SUPER_ADMIN" || r.Action != "payment_account.update" {
		t.Fatalf("row = %+v", r)
	}
	if strings.Contains(string(r.After), "hunter2") || !strings.Contains(string(r.After), "[redacted]") {
		t.Fatalf("secret leaked: %s", r.After)
	}
	if !strings.Contains(string(r.Before), "+94771234567") {
		t.Fatalf("the audit trail must keep the facts it exists for: %s", r.Before)
	}
}

func TestSystemActor(t *testing.T) {
	env := testdb.New(t)
	write(t, env, db.System(), audit.Entry{Action: "timer.expire", Entity: "order", EntityID: "o1"})
	r := list(t, env, audit.Query{}).Data[0]
	if r.ActorID != nil || r.ActorRole != "SYSTEM" || r.Before != nil {
		t.Fatalf("row = %+v", r)
	}
}

// The application role may insert and read but never change or delete a row.
func TestAppendOnly(t *testing.T) {
	env := testdb.New(t)
	write(t, env, db.System(), audit.Entry{Action: "x", Entity: "e"})
	for _, stmt := range []string{`UPDATE audit_log SET action = 'y'`, `DELETE FROM audit_log`} {
		err := env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
			_, err := tx.Exec(ctx, stmt)
			return err
		})
		if err == nil {
			t.Errorf("%q was allowed for the app role", stmt)
		}
	}
	if _, err := env.Owner.Exec(context.Background(), `DELETE FROM audit_log`); err == nil {
		t.Error("even the owner must not delete audit rows (trigger)")
	}
}

func TestListFiltersAndPages(t *testing.T) {
	env := testdb.New(t)
	for i := 0; i < audit.PageSize+5; i++ {
		write(t, env, db.System(), audit.Entry{Action: "shop.update", Entity: "shop", EntityID: fmt.Sprint("s", i%2)})
	}
	write(t, env, db.System(), audit.Entry{Action: "user.block", Entity: "user", EntityID: "u1"})

	if p := list(t, env, audit.Query{Entity: "user"}); len(p.Data) != 1 || p.NextCursor != "" {
		t.Fatalf("entity filter = %+v", p)
	}
	if p := list(t, env, audit.Query{Entity: "shop", EntityID: "s1"}); len(p.Data) != (audit.PageSize+5)/2 {
		t.Fatalf("entity id filter returned %d", len(p.Data))
	}

	first := list(t, env, audit.Query{})
	if len(first.Data) != audit.PageSize || first.NextCursor == "" || first.Data[0].Entity != "user" {
		t.Fatalf("first page = %d rows, cursor %q, newest %q", len(first.Data), first.NextCursor, first.Data[0].Entity)
	}
	cur, err := audit.ParseCursor(first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	second := list(t, env, audit.Query{Cursor: cur})
	if len(second.Data) != 6 || second.NextCursor != "" {
		t.Fatalf("second page = %d rows, cursor %q", len(second.Data), second.NextCursor)
	}
	if second.Data[0].ID >= first.Data[len(first.Data)-1].ID {
		t.Fatal("pages overlap")
	}
}

func TestParseCursor(t *testing.T) {
	for _, bad := range []string{"0", "-1", "abc", "1x", "01"} {
		if _, err := audit.ParseCursor(bad); err == nil {
			t.Errorf("cursor %q accepted", bad)
		}
	}
	if n, err := audit.ParseCursor("12"); err != nil || n != 12 {
		t.Fatalf("got %d, %v", n, err)
	}
}
