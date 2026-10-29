package delivery

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// What the order module asks of delivery, inside its own transaction.

// OpenSlots are the windows a customer can pick in the next maxPublicDays
// Colombo days for a shop the caller has already found.
func (s *Service) OpenSlots(ctx context.Context, tx *db.Tx, shopID uuid.UUID, now time.Time) ([]PublicSlot, error) {
	return openSlots(ctx, tx, shopID, now, startOfColomboDay(now).AddDate(0, 0, maxPublicDays))
}

// CheckSlot answers what Book would answer, without taking a place. A quote
// uses it to tell the customer early; the submit still books with Book, which
// is the check that counts. NOT_FOUND when the slot is not this shop's,
// SLOT_CLOSED when it is switched off or past its cutoff, SLOT_FULL when no place is left.
func (s *Service) CheckSlot(ctx context.Context, tx *db.Tx, shopID, slotID uuid.UUID, now time.Time) error {
	var status enum.SlotStatus
	var cutoff time.Time
	var left int
	err := tx.QueryRow(ctx, `SELECT status, cutoff_at, capacity - booked FROM slots WHERE id = $1 AND shop_id = $2`, slotID, shopID).
		Scan(&status, &cutoff, &left)
	switch {
	case db.IgnoreNoRows(err) != nil:
		return err
	case err != nil:
		return apperr.New(apperr.NotFound)
	case status != enum.SlotStatusOpen || !cutoff.After(now):
		return apperr.New(apperr.SlotClosed)
	case left <= 0:
		return apperr.New(apperr.SlotFull)
	}
	return nil
}

// BookSlot takes one place in a slot of the shop (see Book). It is the order
// module's way in; the check and the increment are one statement.
func (s *Service) BookSlot(ctx context.Context, tx *db.Tx, shopID, slotID uuid.UUID, now time.Time) error {
	return Book(ctx, tx, shopID, slotID, now)
}

// ReleaseSlot gives a place back (see Release).
func (s *Service) ReleaseSlot(ctx context.Context, tx *db.Tx, slotID uuid.UUID) error {
	return Release(ctx, tx, slotID)
}
