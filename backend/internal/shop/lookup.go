package shop

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
)

// What other modules ask of a shop, inside their own transaction.

// CanTakeOrders is for the order module, inside its transaction. It returns nil
// when a bill may be submitted now, SHOP_CLOSED when the shop is switched off
// or outside its hours, and SHOP_NOT_READY when its setup is incomplete.
func (s *Service) CanTakeOrders(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time) error {
	snap, found, err := snapshotOf(ctx, tx, id, now, s.manualEnabled)
	if err != nil {
		return err
	}
	if !found {
		return apperr.New(apperr.NotFound)
	}
	switch snap.canTakeOrders(now) {
	case Closed:
		return apperr.New(apperr.ShopClosed)
	case NotReady:
		return apperr.New(apperr.ShopNotReady)
	}
	return nil
}

// Exists lets other modules (delivery) check a shop id inside their own transaction.
func (s *Service) Exists(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error) {
	return existsShop(ctx, tx, id)
}

// IDBySlug resolves the slug of an ACTIVE shop; suspended and draft shops are not found.
func (s *Service) IDBySlug(ctx context.Context, tx *db.Tx, slug string) (uuid.UUID, bool, error) {
	return activeIDBySlug(ctx, tx, slug)
}

// ActiveLocations is where each active shop is, for "which shops deliver to this pin".
func (s *Service) ActiveLocations(ctx context.Context, tx *db.Tx) ([]Location, error) {
	return activeLocations(ctx, tx)
}

// MarkupBP is the shop's default markup, for the pricing module inside its own
// transaction. found is false for an unknown shop.
func (s *Service) MarkupBP(ctx context.Context, tx *db.Tx, id uuid.UUID) (bp int, found bool, err error) {
	return markupOf(ctx, tx, id)
}
