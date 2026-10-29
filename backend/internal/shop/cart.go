package shop

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/cartclient"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
)

// The cart tenant of a shop: one tenant in the cart service per shop (docs/09 section 1).

// Carts is the part of the cart service this module needs. cartclient.Client implements it.
type Carts interface {
	EnsureTenant(ctx context.Context, externalID, name string) (tenantID, publicKey string, err error)
}

// PriceSyncer queues a full price list upload for a shop. The catalog module
// implements it; it is set after both services exist.
type PriceSyncer interface {
	QueueFullSync(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (queued int, err error)
}

// SetCarts wires the cart service client and the price syncer. Without them,
// EnableCart answers CART_UNAVAILABLE (tests of other modules).
func (s *Service) SetCarts(c Carts, sync PriceSyncer) { s.carts, s.sync = c, sync }

// EnableCart creates the shop's cart tenant and starts the first price upload.
// Calling it again changes nothing and returns the shop as it is.
//
// The cart service is called between two transactions, never inside one: a slow
// network call must not hold a database lock. The tenant is found again by the
// shop id, so a repeat after a failed second step reaches the same tenant.
func (s *Service) EnableCart(ctx context.Context, scope db.Scope, id uuid.UUID) (Detail, error) {
	if s.carts == nil || s.sync == nil {
		return Detail{}, apperr.New(apperr.CartUnavailable)
	}
	var name string
	var enabled bool
	var d Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		sh, found, err := shopByID(ctx, tx, id, false)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		name = sh.Name
		if d, err = s.detail(ctx, tx, id); err != nil {
			return err
		}
		enabled = d.Settings.CartTenantID != nil
		return nil
	})
	if err != nil || enabled {
		return d, err
	}

	tenantID, publicKey, err := s.carts.EnsureTenant(ctx, id.String(), name)
	if err != nil {
		return Detail{}, cartclient.ToAppError(err)
	}

	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if _, found, err := shopByID(ctx, tx, id, true); err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE shop_settings SET cart_tenant_id = $2, cart_public_key = $3
			WHERE shop_id = $1 AND cart_tenant_id IS NULL`, id, tenantID, publicKey)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 { // a concurrent call that got there first did the rest
			if err := audit.Write(ctx, tx, scope, audit.Entry{
				Action: "shop.cart_tenant.create", Entity: "shop_settings", EntityID: id.String(), ShopID: id,
				After: map[string]string{"cartTenantId": tenantID},
			}); err != nil {
				return err
			}
			if _, err := s.sync.QueueFullSync(ctx, tx, id); err != nil {
				return err
			}
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

// ResyncCart queues the whole price list again, for a shop whose cart tenant
// exists. It repairs a list that drifted, for example after a cart service outage.
func (s *Service) ResyncCart(ctx context.Context, scope db.Scope, id uuid.UUID) (queued int, err error) {
	if s.sync == nil {
		return 0, apperr.New(apperr.CartUnavailable)
	}
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if _, found, err := shopByID(ctx, tx, id, false); err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if _, ok, err := cartTenantOf(ctx, tx, id); err != nil {
			return err
		} else if !ok {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"cart": "not_enabled"}}
		}
		queued, err = s.sync.QueueFullSync(ctx, tx, id)
		return err
	})
	return queued, err
}

// CartTenant is the shop's cart tenant id, for the price sync inside its own
// transaction. ok is false while the cart is not enabled.
func (s *Service) CartTenant(ctx context.Context, tx *db.Tx, id uuid.UUID) (tenantID string, ok bool, err error) {
	return cartTenantOf(ctx, tx, id)
}

func cartTenantOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (string, bool, error) {
	var t *string
	err := tx.QueryRow(ctx, `SELECT cart_tenant_id FROM shop_settings WHERE shop_id = $1`, id).Scan(&t)
	if err != nil || t == nil {
		return "", false, db.IgnoreNoRows(err)
	}
	return *t, true, nil
}
