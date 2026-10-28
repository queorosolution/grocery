package db_test

import (
	"context"
	"strings"
	"testing"

	"gp/internal/shared/db"
	"gp/test/testdb"
)

// INFRA-05: the application must refuse to run as the database owner, which skips RLS.
func TestCheckRole(t *testing.T) {
	env := testdb.New(t)
	ctx := context.Background()

	if err := db.CheckRole(ctx, env.DB.Pool(), false); err != nil {
		t.Fatalf("app_rw must pass: %v", err)
	}
	err := db.CheckRole(ctx, env.Owner, false)
	if err == nil || !strings.Contains(err.Error(), db.AppRole) {
		t.Fatalf("the owner must be refused outside local, got %v", err)
	}
	if err := db.CheckRole(ctx, env.Owner, true); err != nil {
		t.Fatalf("local runs may use any role: %v", err)
	}
}

func TestOpenLocalConnectsAsAppRole(t *testing.T) {
	env := testdb.New(t)
	var user string
	if err := env.DB.Pool().QueryRow(context.Background(), `SELECT current_user`).Scan(&user); err != nil || user != db.AppRole {
		t.Fatalf("connected as %q, %v", user, err)
	}
}

func TestOpenRefusesWeakTLS(t *testing.T) {
	for _, mode := range []string{"disable", "allow", "prefer"} {
		_, err := db.Open(context.Background(), db.Options{URL: "postgresql://app_rw:x@127.0.0.1:1/gp?sslmode=" + mode})
		if err == nil || !strings.Contains(err.Error(), "sslmode=require") {
			t.Errorf("sslmode=%s accepted: %v", mode, err)
		}
	}
	// A required-TLS URL passes this check and only then fails to connect.
	_, err := db.Open(context.Background(), db.Options{URL: "postgresql://app_rw:x@127.0.0.1:1/gp?sslmode=require"})
	if err == nil || strings.Contains(err.Error(), "sslmode=require") {
		t.Errorf("sslmode=require should get past the TLS check: %v", err)
	}
}

func TestOpenNeverEchoesTheURL(t *testing.T) {
	_, err := db.Open(context.Background(), db.Options{URL: "postgresql://app_rw:hunter2@bad host/gp"})
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error must not contain the password: %v", err)
	}
}
