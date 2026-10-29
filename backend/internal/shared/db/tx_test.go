package db_test

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/test/testdb"
)

// Same seed ids as contracts/schema_test.sql.
var (
	custA  = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
	custB  = uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
	ownerA = uuid.MustParse("00000000-0000-0000-0000-0000000000a2")
	shopA  = uuid.MustParse("00000000-0000-0000-0000-00000000aaaa")
	shopB  = uuid.MustParse("00000000-0000-0000-0000-00000000bbbb")
)

// seed adds two shops, two customers and one order each, as the owner (RLS skipped).
func seed(t *testing.T, env *testdb.Env) {
	t.Helper()
	_, err := env.Owner.Exec(context.Background(), `
INSERT INTO users (id, google_sub, email, name) VALUES
 ('`+custA.String()+`','g-a1','custa@x.lk','Cust A'),
 ('`+custB.String()+`','g-b1','custb@x.lk','Cust B'),
 ('`+ownerA.String()+`','g-a2','ownera@x.lk','Owner A');
INSERT INTO shops (id, slug, name, lat, lng, status) VALUES
 ('`+shopA.String()+`','krishna-super','Krishna Super',7.29,80.63,'ACTIVE'),
 ('`+shopB.String()+`','other-shop','Other Shop',7.30,80.64,'ACTIVE');
INSERT INTO memberships (user_id, scope, shop_id, role) VALUES ('`+ownerA.String()+`','SHOP','`+shopA.String()+`','SHOP_OWNER');
INSERT INTO slots (id, shop_id, starts_at, ends_at, cutoff_at, capacity) VALUES
 ('00000000-0000-0000-0000-0000000051a1','`+shopA.String()+`', now()+interval '5 hour', now()+interval '7 hour', now()+interval '3 hour', 5),
 ('00000000-0000-0000-0000-0000000051b1','`+shopB.String()+`', now()+interval '5 hour', now()+interval '7 hour', now()+interval '3 hour', 5);
INSERT INTO orders (code, shop_id, customer_id, status, delivery_type, address, lat, lng, distance_m, weight_g, slot_id,
  items_cents, delivery_fee_cents, total_cents, payment_provider) VALUES
 ('KS-AAAA01','`+shopA.String()+`','`+custA.String()+`','SUBMITTED','SLOT','{}',7.29,80.63,1200,5000,'00000000-0000-0000-0000-0000000051a1',143500,15000,158500,'PAYHERE'),
 ('OS-BBBB01','`+shopB.String()+`','`+custB.String()+`','SUBMITTED','SLOT','{}',7.30,80.64,1200,5000,'00000000-0000-0000-0000-0000000051b1',142000,15000,157000,'PAYHERE');`)
	if err != nil {
		t.Fatal(err)
	}
}

func orderCodes(t *testing.T, d *db.DB, scope db.Scope) []string {
	t.Helper()
	var codes []string
	err := d.Tx(context.Background(), scope, func(ctx context.Context, tx *db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT code FROM orders ORDER BY code`)
		if err != nil {
			return err
		}
		codes, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return codes
}

func TestRowLevelSecurityThroughTx(t *testing.T) {
	env := testdb.New(t)
	seed(t, env)
	cases := []struct {
		name  string
		scope db.Scope
		want  string // codes joined by ","
	}{
		{"customer sees only own order", db.Scope{UserID: custA, Role: enum.RoleCustomer}, "KS-AAAA01"},
		{"other customer sees only theirs", db.Scope{UserID: custB, Role: enum.RoleCustomer}, "OS-BBBB01"},
		{"shop owner sees own shop only", db.Scope{UserID: ownerA, ShopID: shopA, Role: enum.RoleShopOwner}, "KS-AAAA01"},
		{"public sees nothing", db.Public(), ""},
		{"system sees everything", db.System(), "KS-AAAA01,OS-BBBB01"},
	}
	for _, tc := range cases {
		if got := strings.Join(orderCodes(t, env.DB, tc.scope), ","); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The scope is set with set_config(..., true), so it must not survive the transaction.
func TestScopeDoesNotLeakToTheNextTransaction(t *testing.T) {
	env := testdb.New(t)
	seed(t, env)
	for i := 0; i < 6; i++ { // more than the 2 pooled connections, so each one is reused
		if got := orderCodes(t, env.DB, db.System()); len(got) != 2 {
			t.Fatalf("system scope lost: %v", got)
		}
		if got := orderCodes(t, env.DB, db.Public()); len(got) != 0 {
			t.Fatalf("round %d: a public transaction saw %v after a privileged one", i, got)
		}
	}
}

func TestConcurrentTransactionsKeepTheirOwnScope(t *testing.T) {
	env := testdb.New(t)
	seed(t, env)
	var wg sync.WaitGroup
	errs := make(chan string, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scope, want := db.Scope{UserID: custA}, 1
			if i%2 == 0 {
				scope, want = db.Scope{UserID: custB}, 1
			}
			if i%5 == 0 {
				scope, want = db.Public(), 0
			}
			var n int
			err := env.DB.Tx(context.Background(), scope, func(ctx context.Context, tx *db.Tx) error {
				return tx.QueryRow(ctx, `SELECT count(*) FROM orders`).Scan(&n)
			})
			if err != nil || n != want {
				errs <- "wrong view"
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	if len(errs) > 0 {
		t.Fatalf("%d transactions saw the wrong rows", len(errs))
	}
}

func TestCommitAndRollback(t *testing.T) {
	env := testdb.New(t)
	ctx := context.Background()
	insert := func(ctx context.Context, tx *db.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO categories (slug, name) VALUES ('rice','Rice')`)
		return err
	}
	count := func() int {
		var n int
		env.Owner.QueryRow(ctx, `SELECT count(*) FROM categories`).Scan(&n)
		return n
	}

	boom := errors.New("boom")
	err := env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if err := insert(ctx, tx); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || count() != 0 {
		t.Fatalf("an error must roll back: err=%v rows=%d", err, count())
	}

	func() {
		defer func() { recover() }()
		env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
			insert(ctx, tx)
			panic("bug")
		})
	}()
	if count() != 0 {
		t.Fatal("a panic must roll back")
	}

	if err := env.DB.Tx(ctx, db.System(), insert); err != nil || count() != 1 {
		t.Fatalf("commit failed: %v rows=%d", err, count())
	}
}

func TestPanicIsNotSwallowed(t *testing.T) {
	env := testdb.New(t)
	defer func() {
		if recover() != "bug" {
			t.Fatal("Tx must re-panic with the original value")
		}
	}()
	env.DB.Tx(context.Background(), db.Public(), func(context.Context, *db.Tx) error { panic("bug") })
}

// A callback that ends its goroutine (t.Fatal does) must not leave the
// connection inside an open transaction, or closing the pool would hang.
func TestGoexitRollsBack(t *testing.T) {
	env := testdb.New(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO users (google_sub, email, name) VALUES ('g', 'g@example.test', 'G')`); err != nil {
				t.Error(err)
			}
			runtime.Goexit()
			return nil
		})
	}()
	<-done
	var n, open int
	if err := env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows = %d, err %v", n, err)
	}
	if err := env.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND state LIKE 'idle in transaction%'`).Scan(&open); err != nil || open != 0 {
		t.Fatalf("open transactions = %d, err %v", open, err)
	}
}

func TestAfterCommit(t *testing.T) {
	env := testdb.New(t)
	ctx := context.Background()
	var ran []string

	err := env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		tx.AfterCommit(func(context.Context) error { ran = append(ran, "first"); return nil })
		tx.AfterCommit(func(context.Context) error { ran = append(ran, "second"); return errors.New("sqs down") })
		tx.AfterCommit(func(context.Context) error { ran = append(ran, "third"); return nil })
		if len(ran) != 0 {
			t.Error("a hook ran before commit")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("a failing hook must not fail a committed transaction: %v", err)
	}
	if strings.Join(ran, ",") != "first,second,third" {
		t.Fatalf("hooks ran as %v, want all three in order", ran)
	}

	ran = nil
	env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		tx.AfterCommit(func(context.Context) error { ran = append(ran, "never"); return nil })
		return errors.New("rolled back")
	})
	if len(ran) != 0 {
		t.Fatal("a hook ran after a rollback: an event would be published for data that does not exist")
	}
}

func TestAfterCommitSurvivesCancelledRequest(t *testing.T) {
	env := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	var hookErr error
	env.DB.Tx(ctx, db.System(), func(_ context.Context, tx *db.Tx) error {
		tx.AfterCommit(func(hookCtx context.Context) error {
			cancel() // the client hung up
			hookErr = hookCtx.Err()
			return nil
		})
		return nil
	})
	if hookErr != nil {
		t.Fatalf("hook context must not be cancelled with the request: %v", hookErr)
	}
}
