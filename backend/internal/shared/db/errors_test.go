package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestErrorHelpers(t *testing.T) {
	wrap := func(code string) error { return fmt.Errorf("x: %w", &pgconn.PgError{Code: code}) }
	if !IsUniqueViolation(wrap("23505")) || IsUniqueViolation(wrap("23514")) || IsUniqueViolation(errors.New("x")) {
		t.Error("IsUniqueViolation")
	}
	if !IsCheckViolation(wrap("23514")) || IsCheckViolation(wrap("23505")) {
		t.Error("IsCheckViolation")
	}
	if IgnoreNoRows(pgx.ErrNoRows) != nil || IgnoreNoRows(errors.New("boom")) == nil {
		t.Error("IgnoreNoRows")
	}
}
