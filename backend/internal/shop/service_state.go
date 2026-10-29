package shop

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
)

// The live state of a shop: its weekly hours and the accepting switch.

// SetHours replaces the weekly hours. An empty list means closed every day.
func (s *Service) SetHours(ctx context.Context, scope db.Scope, id uuid.UUID, days []Day) (Detail, error) {
	week, err := parseHours(days)
	if err != nil {
		return Detail{}, err
	}
	var d Detail
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if _, found, err := shopByID(ctx, tx, id, true); err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		before, err := hoursOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := replaceHours(ctx, tx, id, week); err != nil {
			return err
		}
		after, err := hoursOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{
			Action: "shop.hours.update", Entity: "shop_hours", EntityID: id.String(), ShopID: id,
			Before: before, After: after,
		}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

// SetAccepting switches order taking on or off. Switching on is refused with
// SHOP_NOT_READY and the list of what is missing; off always works.
func (s *Service) SetAccepting(ctx context.Context, scope db.Scope, id uuid.UUID, on bool) (Detail, error) {
	var d Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := shopByID(ctx, tx, id, true)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if on {
			r, err := s.readiness(ctx, tx, id)
			if err != nil {
				return err
			}
			if !r.Ready {
				fields := map[string]string{}
				for _, m := range r.Missing {
					fields[string(m)] = "missing"
				}
				return &apperr.Error{Code: apperr.ShopNotReady, Fields: fields}
			}
		}
		if err := setAccepting(ctx, tx, id, on); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{
			Action: "shop.accepting", Entity: "shop", EntityID: id.String(), ShopID: id,
			Before: map[string]bool{"acceptingOrders": before.AcceptingOrders}, After: map[string]bool{"acceptingOrders": on},
		}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}
