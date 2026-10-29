// Package testdb gives a test its own real Postgres database with the project
// schema applied by the same migrations production uses. Mocking the database
// would hide exactly the bugs that row-level security exists to catch.
//
// It needs a Postgres where TEST_DATABASE_URL (default: postgres/postgres on
// localhost) may create databases and roles. If none is reachable the test is
// skipped, unless CI or REQUIRE_TEST_DB is set: then it fails, so a broken CI
// database never passes silently.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"gp/internal/shared/db"
	"gp/migrations"
)

const (
	defaultAdminURL = "postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable" // #nosec G101 -- local test database
	appPassword     = "app_rw"                                                                 // #nosec G101 -- local test role
	// One lock for role and database setup: concurrent packages would otherwise
	// race on creating the shared app_rw role.
	setupLock = 7234001
)

type Env struct {
	Name     string
	Owner    *pgxpool.Pool // the database owner: skips RLS, use it only to seed and inspect
	DB       *db.DB        // connected as app_rw, like the application
	OwnerURL string
	AppURL   string
}

// New creates a fresh database for the test and drops it when the test ends.
func New(t testing.TB) *Env {
	t.Helper()
	ctx := context.Background()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		adminURL = defaultAdminURL
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err == nil {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		err = admin.Ping(pingCtx)
	}
	if err != nil {
		if os.Getenv("CI") != "" || os.Getenv("REQUIRE_TEST_DB") != "" {
			t.Fatalf("testdb: no Postgres at TEST_DATABASE_URL: %v", err)
		}
		t.Skip("testdb: no Postgres reachable; set TEST_DATABASE_URL to run database tests")
	}

	name := "gp_test_" + randomHex()
	conn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", setupLock); err != nil {
		t.Fatal(err)
	}
	ownerURL, appURL := withDatabase(adminURL, name, "", ""), ""
	setup := func() error {
		if _, err := conn.Exec(ctx, `CREATE DATABASE `+name); err != nil {
			return err
		}
		if err := migrate(ctx, ownerURL); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, `ALTER ROLE app_rw LOGIN PASSWORD '`+appPassword+`'`); err != nil {
			return err
		}
		appURL = withDatabase(adminURL, name, "app_rw", appPassword)
		return nil
	}
	setupErr := setup()
	_, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", setupLock)
	conn.Release()
	if setupErr != nil {
		t.Fatalf("testdb: setup: %v", setupErr)
	}

	owner, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	appDB, err := db.Open(ctx, db.Options{URL: appURL, Local: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		appDB.Close()
		owner.Close()
		_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
		admin.Close()
	})
	return &Env{Name: name, Owner: owner, DB: appDB, OwnerURL: ownerURL, AppURL: appURL}
}

func migrate(ctx context.Context, ownerURL string) error {
	sqlDB, err := sql.Open("pgx", ownerURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	p, err := migrations.Provider(sqlDB)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

// withDatabase swaps the database name (and optionally the login) in a URL.
func withDatabase(raw, name, user, password string) string {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err) // a malformed TEST_DATABASE_URL is a setup error
	}
	u.Path = "/" + name
	if user != "" {
		u.User = url.UserPassword(user, password)
	}
	return u.String()
}

func randomHex() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
