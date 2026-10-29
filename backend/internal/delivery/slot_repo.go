package delivery

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

const slotColumns = `id, shop_id, starts_at, ends_at, cutoff_at, capacity, booked, status`

func scanSlot(row pgx.Row) (Slot, error) {
	var s Slot
	err := row.Scan(&s.ID, &s.ShopID, &s.StartsAt, &s.EndsAt, &s.CutoffAt, &s.Capacity, &s.Booked, &s.Status)
	return s, err
}

func collectSlots(rows pgx.Rows) ([]Slot, error) {
	defer rows.Close()
	out := []Slot{}
	for rows.Next() {
		s, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// slotsBetween lists a shop's slots with from <= starts_at < to.
func slotsBetween(ctx context.Context, tx *db.Tx, shopID uuid.UUID, from, to time.Time) ([]Slot, error) {
	rows, err := tx.Query(ctx, `SELECT `+slotColumns+` FROM slots
		WHERE shop_id = $1 AND starts_at >= $2 AND starts_at < $3 ORDER BY starts_at`, shopID, from, to)
	if err != nil {
		return nil, err
	}
	return collectSlots(rows)
}

// insertSlot returns CONFLICT when the shop already has a slot at that start.
func insertSlot(ctx context.Context, tx *db.Tx, shopID uuid.UUID, in SlotInput) (Slot, error) {
	s, err := scanSlot(tx.QueryRow(ctx, `
		INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+slotColumns,
		shopID, in.StartsAt, in.EndsAt, in.CutoffAt, in.Capacity))
	if db.IsUniqueViolation(err) {
		return Slot{}, apperr.New(apperr.Conflict)
	}
	return s, err
}

func slotByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (Slot, bool, error) {
	s, err := scanSlot(tx.QueryRow(ctx, `SELECT `+slotColumns+` FROM slots WHERE id = $1 FOR UPDATE`, id))
	return s, err == nil, db.IgnoreNoRows(err)
}

func patchSlot(ctx context.Context, tx *db.Tx, id uuid.UUID, p SlotPatch) (Slot, error) {
	s, err := scanSlot(tx.QueryRow(ctx, `
		UPDATE slots SET capacity = COALESCE($2, capacity), status = COALESCE($3, status),
			cutoff_at = COALESCE($4, cutoff_at)
		WHERE id = $1 RETURNING `+slotColumns, id, p.Capacity, p.Status, p.CutoffAt))
	if db.IsCheckViolation(err) { // cutoff after the start
		return Slot{}, apperr.Validation(map[string]string{"cutoffAt": "after_start"})
	}
	return s, err
}

// countSlots is used to stop a copy that would be too big.
func countSlots(ctx context.Context, tx *db.Tx, shopID uuid.UUID, from, to time.Time) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM slots WHERE shop_id = $1 AND starts_at >= $2 AND starts_at < $3`,
		shopID, from, to).Scan(&n)
	return n, err
}

// copyForward copies the slots of [from, to) seven days ahead as empty, open
// slots. A start that already has a slot is skipped, and so is one that would
// already be in the past. 168 hours is exact because Colombo has no daylight saving.
func copyForward(ctx context.Context, tx *db.Tx, shopID uuid.UUID, from, to, now time.Time) ([]Slot, error) {
	rows, err := tx.Query(ctx, `
		INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity)
		SELECT shop_id, starts_at + interval '168 hours', ends_at + interval '168 hours',
		       cutoff_at + interval '168 hours', capacity
		FROM slots
		WHERE shop_id = $1 AND starts_at >= $2 AND starts_at < $3 AND starts_at + interval '168 hours' > $4
		ON CONFLICT (shop_id, starts_at) DO NOTHING
		RETURNING `+slotColumns, shopID, from, to, now)
	if err != nil {
		return nil, err
	}
	out, err := collectSlots(rows)
	// RETURNING has no defined order.
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out, err
}

// openSlots are the windows a customer can still choose: open, before the
// cutoff, and starting before end. A full slot is included with remaining 0.
func openSlots(ctx context.Context, tx *db.Tx, shopID uuid.UUID, now, end time.Time) ([]PublicSlot, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, starts_at, ends_at, cutoff_at, GREATEST(capacity - booked, 0) FROM slots
		WHERE shop_id = $1 AND status = $2 AND cutoff_at > $3 AND starts_at < $4 ORDER BY starts_at`,
		shopID, enum.SlotStatusOpen, now, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublicSlot{}
	for rows.Next() {
		var s PublicSlot
		if err := rows.Scan(&s.ID, &s.StartsAt, &s.EndsAt, &s.CutoffAt, &s.Remaining); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Book takes one place in a slot, inside the caller's transaction (T01, T03).
// The check and the increment are one UPDATE, so two buyers can never take the
// last place. Failures say why: SLOT_FULL, SLOT_CLOSED (switched off or past
// the cutoff), or NOT_FOUND when the slot is not this shop's.
func Book(ctx context.Context, tx *db.Tx, shopID, slotID uuid.UUID, now time.Time) error {
	tag, err := tx.Exec(ctx, `
		UPDATE slots SET booked = booked + 1
		WHERE id = $1 AND shop_id = $2 AND status = $3 AND booked < capacity AND cutoff_at > $4`,
		slotID, shopID, enum.SlotStatusOpen, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var status enum.SlotStatus
	var cutoff time.Time
	err = tx.QueryRow(ctx, `SELECT status, cutoff_at FROM slots WHERE id = $1 AND shop_id = $2`, slotID, shopID).
		Scan(&status, &cutoff)
	switch {
	case db.IgnoreNoRows(err) != nil:
		return err
	case err != nil:
		return apperr.New(apperr.NotFound)
	case status != enum.SlotStatusOpen || !cutoff.After(now):
		return apperr.New(apperr.SlotClosed)
	}
	return apperr.New(apperr.SlotFull)
}

// Release gives one place back (T04, T05, T06, ... docs/03 section 10). A slot
// with nothing booked means a place is released twice; that is a bug, so it is
// an error and the order transaction rolls back.
func Release(ctx context.Context, tx *db.Tx, slotID uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE slots SET booked = booked - 1 WHERE id = $1 AND booked > 0`, slotID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("delivery: release slot %s: no place is booked", slotID)
	}
	return nil
}
