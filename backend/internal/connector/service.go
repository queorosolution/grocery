package connector

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"gp/internal/audit"
	"gp/internal/connector/source"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
	"gp/internal/shared/validate"
)

// opTimeout bounds one admin action that talks to the shop's database.
const opTimeout = 30 * time.Second

type Service struct {
	db      *db.DB
	secrets secrets.Store
	src     source.Opener
	imgBase string // IMG_BASE_URL: source image links are kept only under it
}

func NewService(d *db.DB, sec secrets.Store, src source.Opener, imgBaseURL string) *Service {
	return &Service{db: d, secrets: sec, src: src, imgBase: imgBaseURL}
}

// Create saves the connection string of a shop's database. A shop has one
// connector; use ReplaceSecret to change the string. The string goes to the
// secret store first, so a failed write leaves no connector without a secret.
func (s *Service) Create(ctx context.Context, scope db.Scope, shopID uuid.UUID, in SecretInput) (Connector, error) {
	target, err := parseString(in.ConnectionString)
	if err != nil {
		return Connector{}, err
	}
	id := uuid.New()
	var out Connector
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM shops WHERE id = $1)`, shopID).Scan(&exists); err != nil || !exists {
			return apperr.NotFoundUnless(err, exists)
		}
		if _, found, err := byShop(ctx, tx, shopID); err != nil {
			return err
		} else if found {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"shopId": "has_connector"}}
		}
		ref := secretPrefix + id.String()
		if err := s.secrets.Put(ctx, ref, strings.TrimSpace(in.ConnectionString)); err != nil {
			return apperr.New(apperr.Internal).Wrap(err)
		}
		if out, err = insert(ctx, tx, id, shopID, ref, target.Display()); err != nil {
			if db.IsUniqueViolation(err) { // two requests for the same shop at once
				return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"shopId": "has_connector"}}
			}
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "connector.create", Entity: "connector", EntityID: id.String(), ShopID: shopID,
			After: map[string]string{"host": out.HostDisplay},
		})
	})
	return out, err
}

// ReplaceSecret swaps the connection string, for example after the shop changed
// its database password. The mapping is kept; the next test or preview shows
// whether it still fits.
func (s *Service) ReplaceSecret(ctx context.Context, scope db.Scope, id uuid.UUID, in SecretInput) (Connector, error) {
	target, err := parseString(in.ConnectionString)
	if err != nil {
		return Connector{}, err
	}
	var out Connector
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := byID(ctx, tx, id, true)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		ref, err := secretRefOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.secrets.Put(ctx, ref, strings.TrimSpace(in.ConnectionString)); err != nil {
			return apperr.New(apperr.Internal).Wrap(err)
		}
		if out, err = setDisplay(ctx, tx, id, target.Display()); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "connector.secret.replace", Entity: "connector", EntityID: id.String(), ShopID: before.ShopID,
			Before: map[string]string{"host": before.HostDisplay}, After: map[string]any{"host": out.HostDisplay, "secretReplaced": true},
		})
	})
	return out, err
}

// parseString turns a bad string into a validation error that never repeats the string.
func parseString(raw string) (source.Target, error) {
	t, err := source.ParseConnString(raw)
	if err != nil {
		return source.Target{}, validate.New().Check("connectionString", false, "invalid").Err()
	}
	return t, nil
}

// ForShop is the shop's connector, or NOT_FOUND when it has none.
func (s *Service) ForShop(ctx context.Context, scope db.Scope, shopID uuid.UUID) (Connector, error) {
	var out Connector
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		var found bool
		out, found, err = byShop(ctx, tx, shopID)
		return apperr.NotFoundUnless(err, found)
	})
	return out, err
}

// with opens the shop's database for one action and closes it again. The
// database transaction is over before the network call starts. Anything that
// goes wrong on the way out is CONNECTOR_TEST_FAILED with one fixed message: the
// reason (a refused address, a wrong password, a missing table) is only in the
// log, so the endpoint cannot be used to probe the network (SSRF-02).
func (s *Service) with(ctx context.Context, scope db.Scope, id uuid.UUID, op string, fn func(context.Context, Connector, source.Conn) error) error {
	var c Connector
	var ref string
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		var found bool
		var err error
		if c, found, err = byID(ctx, tx, id, false); err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		ref, err = secretRefOf(ctx, tx, id)
		return err
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	connString, err := s.secrets.Get(ctx, ref)
	if err != nil {
		return failed(ctx, op, "secret", err)
	}
	conn, err := s.src.Open(ctx, connString)
	if err != nil {
		return failed(ctx, op, "open", err)
	}
	defer conn.Close()
	err = fn(ctx, c, conn)
	var app *apperr.Error
	if err == nil || errors.As(err, &app) {
		return err
	}
	return failed(ctx, op, "query", err)
}

// failed logs where it went wrong (no host, no user, no message from the other
// database) and answers with the fixed error.
func failed(ctx context.Context, op, stage string, err error) error {
	attrs := []any{"op", op, "stage", stage}
	var oe *source.OpenError
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &oe):
		attrs = append(attrs, "open_stage", oe.Stage, "sqlstate", oe.Code)
	case errors.As(err, &pe):
		attrs = append(attrs, "sqlstate", pe.Code)
	case errors.Is(err, context.DeadlineExceeded):
		attrs = append(attrs, "timeout", true)
	}
	logx.From(ctx).Warn("connector.source_failed", attrs...)
	return apperr.New(apperr.ConnectorTestFailed)
}

// Test connects and reports the server version and whether the login can write.
func (s *Service) Test(ctx context.Context, scope db.Scope, id uuid.UUID) (TestResult, error) {
	var res TestResult
	err := s.with(ctx, scope, id, "test", func(ctx context.Context, _ Connector, conn source.Conn) error {
		info, err := conn.Test(ctx)
		if err != nil {
			return err
		}
		res = TestResult{OK: true, ReadOnly: !info.CanWrite, ServerVersion: info.ServerVersion, Warnings: []string{}}
		if info.CanWrite {
			res.Warnings = append(res.Warnings, "The database user can change data. Use a read-only user.")
		}
		if !info.TLS {
			res.Warnings = append(res.Warnings, "The connection is not encrypted.")
		}
		return nil
	})
	return res, err
}

// Tables lists the tables and views the login may read.
func (s *Service) Tables(ctx context.Context, scope db.Scope, id uuid.UUID) ([]source.Table, error) {
	var out []source.Table
	err := s.with(ctx, scope, id, "tables", func(ctx context.Context, _ Connector, conn source.Conn) (err error) {
		out, err = conn.Tables(ctx)
		return err
	})
	return out, err
}

// Columns lists the columns of one table. An unknown table is NOT_FOUND.
func (s *Service) Columns(ctx context.Context, scope db.Scope, id uuid.UUID, table string) ([]source.Column, error) {
	t, ok := source.ParseTable(table)
	if !ok {
		return nil, validate.New().Check("table", false, "invalid").Err()
	}
	var out []source.Column
	err := s.with(ctx, scope, id, "columns", func(ctx context.Context, _ Connector, conn source.Conn) (err error) {
		if out, err = conn.Columns(ctx, t); err == nil && len(out) == 0 {
			err = apperr.New(apperr.NotFound)
		}
		return err
	})
	return out, err
}
