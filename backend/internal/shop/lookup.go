package shop

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
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

// OrderFacts is what the bill builder reads about a shop.
type OrderFacts struct {
	Location               geo.Point
	MinOrderCents          int64
	MinPlatformMarginCents int64
	MaxLines               int
	// CartTenant is empty while the shop's cart is not enabled.
	CartTenant string
	// Open is true when the shop is accepting orders now (CanTakeOrders would pass).
	Open bool
}

// OrderFacts gathers them inside the caller's transaction. A shop that is not
// ACTIVE is not found, so a customer cannot tell a draft shop from a missing one.
func (s *Service) OrderFacts(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time) (OrderFacts, error) {
	var f OrderFacts
	err := tx.QueryRow(ctx, `SELECT lat, lng FROM shops WHERE id = $1 AND status = 'ACTIVE'`, id).Scan(&f.Location.Lat, &f.Location.Lng)
	if err != nil {
		return f, apperr.NotFoundUnless(db.IgnoreNoRows(err), false)
	}
	set, err := settingsOf(ctx, tx, id)
	if err != nil {
		return f, err
	}
	f.MinOrderCents, f.MinPlatformMarginCents, f.MaxLines = set.MinOrderCents, set.MinPlatformMarginCents, set.MaxLines
	if set.CartTenantID != nil {
		f.CartTenant = *set.CartTenantID
	}
	snap, found, err := snapshotOf(ctx, tx, id, now, s.manualEnabled)
	if err != nil || !found {
		return f, apperr.NotFoundUnless(err, found)
	}
	f.Open = snap.canTakeOrders(now) == CanOrder
	return f, nil
}
