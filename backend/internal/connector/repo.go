package connector

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"gp/internal/shared/db"
)

const connectorColumns = `id, shop_id, kind, host_display, source_table, money_unit, change_column, interval_min,
	auto_create_products, mapping, status, trigger_secret_ref IS NOT NULL, last_run_at, last_success_at,
	consecutive_failures, created_at, updated_at`

func scanConnector(row interface{ Scan(...any) error }) (Connector, error) {
	var c Connector
	var mapping []byte
	err := row.Scan(&c.ID, &c.ShopID, &c.Kind, &c.HostDisplay, &c.SourceTable, &c.MoneyUnit, &c.ChangeColumn, &c.IntervalMin,
		&c.AutoCreateProducts, &mapping, &c.Status, &c.HasTriggerSecret, &c.LastRunAt, &c.LastSuccessAt,
		&c.ConsecutiveFailures, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	c.Columns = map[string]string{}
	return c, json.Unmarshal(mapping, &c.Columns)
}

// byID reads a connector, and with lock also holds it until the transaction ends.
func byID(ctx context.Context, tx *db.Tx, id uuid.UUID, lock bool) (Connector, bool, error) {
	q := `SELECT ` + connectorColumns + ` FROM connectors WHERE id = $1`
	if lock {
		q += ` FOR UPDATE`
	}
	c, err := scanConnector(tx.QueryRow(ctx, q, id))
	return c, err == nil, db.IgnoreNoRows(err)
}

func byShop(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (Connector, bool, error) {
	c, err := scanConnector(tx.QueryRow(ctx, `SELECT `+connectorColumns+` FROM connectors WHERE shop_id = $1`, shopID))
	return c, err == nil, db.IgnoreNoRows(err)
}

func secretRefOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (string, error) {
	var ref string
	err := tx.QueryRow(ctx, `SELECT secret_ref FROM connectors WHERE id = $1`, id).Scan(&ref)
	return ref, err
}

func insert(ctx context.Context, tx *db.Tx, id, shopID uuid.UUID, ref, display string) (Connector, error) {
	return scanConnector(tx.QueryRow(ctx, `
		INSERT INTO connectors (id, shop_id, secret_ref, host_display) VALUES ($1, $2, $3, $4)
		RETURNING `+connectorColumns, id, shopID, ref, display))
}

func setDisplay(ctx context.Context, tx *db.Tx, id uuid.UUID, display string) (Connector, error) {
	return scanConnector(tx.QueryRow(ctx, `UPDATE connectors SET host_display = $2 WHERE id = $1 RETURNING `+connectorColumns, id, display))
}

// saveMapping stores the mapping and starts the next run from scratch: the old
// cursor belonged to the old columns.
func saveMapping(ctx context.Context, tx *db.Tx, id uuid.UUID, in MappingInput, changeColumn *string) (Connector, error) {
	raw, err := json.Marshal(in.Columns)
	if err != nil {
		return Connector{}, err
	}
	return scanConnector(tx.QueryRow(ctx, `
		UPDATE connectors SET source_table = $2, money_unit = $3, change_column = $4, interval_min = $5,
		       auto_create_products = $6, mapping = $7::jsonb, last_cursor = NULL
		WHERE id = $1 RETURNING `+connectorColumns,
		id, in.SourceTable, in.MoneyUnit, changeColumn, in.IntervalMin, in.AutoCreateProducts, string(raw)))
}
