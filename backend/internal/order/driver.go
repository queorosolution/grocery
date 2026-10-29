package order

import (
	"context"
	"math"
	"slices"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
	"gp/internal/team"
)

// driverKind reads what the driver is to the shop as the system: a shop only sees
// its own drivers, but a platform driver the admin chose must still count.
func (s *Service) driverKind(ctx context.Context, driverID, shopID uuid.UUID) (kind team.DriverKind, err error) {
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		kind, err = s.drivers.DriverKindOf(ctx, tx, driverID, shopID)
		return err
	})
	return kind, err
}

// driverFor checks the driver against the shop's driver source (FR-DLV-02): a
// shop's own driver when the source is SHOP or BOTH, a platform driver when it is
// PLATFORM or BOTH. adminMay is false for a shop's staff, who never pick a
// platform driver: those are assigned by the admin. It returns the kind.
func (s *Service) driverFor(ctx context.Context, driverID, shopID uuid.UUID, adminMay bool) (team.DriverKind, error) {
	kind, err := s.driverKind(ctx, driverID, shopID)
	if err != nil {
		return kind, err
	}
	var src enum.DriverSource
	if err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		src, err = s.shops.DriverSourceOf(ctx, tx, shopID)
		return err
	}); err != nil {
		return kind, err
	}
	ok := (kind == team.ShopDriver && src != enum.DriverSourcePlatform) ||
		(kind == team.PlatformDriver && adminMay && src != enum.DriverSourceShop)
	if !ok {
		return kind, apperr.Validation(map[string]string{"driver": "not_allowed"})
	}
	return kind, nil
}

// assignable are the statuses in which a driver can be chosen or changed.
var assignable = []enum.OrderStatus{enum.OrderStatusPaid, enum.OrderStatusPicking, enum.OrderStatusPacked, enum.OrderStatusDeliveryFailed}

// AssignInput is the body of the driver routes of the console and the admin.
type AssignInput struct {
	Version  int       `json:"version"`
	DriverID uuid.UUID `json:"driverId"`
}

// AssignDriver sets the order's driver. The shop's staff choose among the shop's
// own drivers; the admin may also choose a platform driver. Either way the
// shop's driver source decides (FR-DLV-02). The version goes up, so a screen that
// showed the old driver cannot send the order out.
func (s *Service) AssignDriver(ctx context.Context, scope db.Scope, orderID uuid.UUID, in AssignInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Check("driverId", in.DriverID != uuid.Nil, "required").Err(); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		o, found, err := lockOrder(ctx, tx, orderID)
		if err != nil || !found || !inScope(scope, o) {
			return apperr.NotFoundUnless(err, found && inScope(scope, o))
		}
		switch {
		case o.Version != in.Version:
			return apperr.New(apperr.StaleOrder)
		case !slices.Contains(assignable, o.Status):
			return apperr.New(apperr.InvalidTransition)
		}
		if _, err := s.driverFor(ctx, in.DriverID, o.ShopID, scope.Privileged); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET driver_id = $2, version = version + 1 WHERE id = $1`, orderID, in.DriverID); err != nil {
			return err
		}
		if scope.Role == enum.RoleSuperAdmin {
			if err := audit.Write(ctx, tx, scope, audit.Entry{Action: "order.assign_driver", Entity: "order", EntityID: orderID.String(), ShopID: o.ShopID,
				Before: map[string]any{"driverId": o.DriverID}, After: map[string]any{"driverId": in.DriverID}}); err != nil {
				return err
			}
		}
		out, err = s.detail(ctx, tx, scope, orderID)
		return err
	})
	return out, err
}

// Drivers lists the shop's own drivers, for the console's driver picker. Platform
// drivers are not listed: the admin assigns them.
func (s *Service) Drivers(ctx context.Context, scope db.Scope) ([]team.Driver, error) {
	var out []team.Driver
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = s.drivers.ShopDrivers(ctx, tx, scope.ShopID)
		return err
	})
	return out, err
}
