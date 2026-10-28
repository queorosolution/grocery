package delivery

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
)

// Distances and weights are stored as numeric(x,2); we move them as hundredths.
const ruleColumns = `shop_id, base_cents, per_km_cents, (free_km * 100)::bigint, per_kg_cents,
	(free_kg * 100)::bigint, free_above_cents, min_cents, max_cents, (max_km * 100)::bigint,
	(road_factor * 100)::bigint, updated_at`

func scanRule(row pgx.Row) (RuleRow, error) {
	var r RuleRow
	err := row.Scan(&r.ShopID, &r.Rule.BaseCents, &r.Rule.PerKmCents, &r.Rule.FreeKm, &r.Rule.PerKgCents,
		&r.Rule.FreeKg, &r.Rule.FreeAboveCents, &r.Rule.MinCents, &r.Rule.MaxCents, &r.Rule.MaxKm,
		&r.Rule.RoadFactor, &r.UpdatedAt)
	return r, err
}

// listRules returns the platform default first, then shop rules by shop id.
func listRules(ctx context.Context, tx *db.Tx) ([]RuleRow, error) {
	rows, err := tx.Query(ctx, `SELECT `+ruleColumns+` FROM delivery_fee_rules ORDER BY shop_id NULLS FIRST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RuleRow{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ruleFor is the shop's own rule when it has one, otherwise the platform default.
func ruleFor(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (RuleRow, bool, error) {
	r, err := scanRule(tx.QueryRow(ctx, `SELECT `+ruleColumns+` FROM delivery_fee_rules
		WHERE shop_id = $1 OR shop_id IS NULL ORDER BY shop_id NULLS LAST LIMIT 1`, shopID))
	return r, err == nil, db.IgnoreNoRows(err)
}

// exactRule reads one rule without falling back (used for the audit "before").
func exactRule(ctx context.Context, tx *db.Tx, shopID *uuid.UUID) (RuleRow, bool, error) {
	r, err := scanRule(tx.QueryRow(ctx, `SELECT `+ruleColumns+` FROM delivery_fee_rules
		WHERE shop_id IS NOT DISTINCT FROM $1`, shopID))
	return r, err == nil, db.IgnoreNoRows(err)
}

// saveRule replaces the rule of a shop (nil = platform default). The unique
// index is on coalesce(shop_id, zero uuid), so the conflict target repeats it.
func saveRule(ctx context.Context, tx *db.Tx, shopID *uuid.UUID, r Rule) (RuleRow, error) {
	return scanRule(tx.QueryRow(ctx, `
		INSERT INTO delivery_fee_rules (shop_id, base_cents, per_km_cents, free_km, per_kg_cents, free_kg,
			free_above_cents, min_cents, max_cents, max_km, road_factor)
		VALUES ($1, $2, $3, $4::bigint / 100.0, $5, $6::bigint / 100.0, $7, $8, $9, $10::bigint / 100.0, $11::bigint / 100.0)
		ON CONFLICT ((coalesce(shop_id, '00000000-0000-0000-0000-000000000000'::uuid))) DO UPDATE SET
			base_cents = EXCLUDED.base_cents, per_km_cents = EXCLUDED.per_km_cents, free_km = EXCLUDED.free_km,
			per_kg_cents = EXCLUDED.per_kg_cents, free_kg = EXCLUDED.free_kg,
			free_above_cents = EXCLUDED.free_above_cents, min_cents = EXCLUDED.min_cents,
			max_cents = EXCLUDED.max_cents, max_km = EXCLUDED.max_km, road_factor = EXCLUDED.road_factor
		RETURNING `+ruleColumns,
		shopID, r.BaseCents, r.PerKmCents, r.FreeKm, r.PerKgCents, r.FreeKg, r.FreeAboveCents,
		r.MinCents, r.MaxCents, r.MaxKm, r.RoadFactor))
}
