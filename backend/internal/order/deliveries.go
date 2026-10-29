package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// What a driver sees of the orders assigned to them (docs/06 section 9): where to
// go and whom to call, never prices per line, the coupon, the payment or the code.

type DriverCustomer struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type DriverAddress struct {
	Label    string  `json:"label"`
	Line1    string  `json:"line1"`
	Landmark *string `json:"landmark"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

type DriverOrder struct {
	ID         uuid.UUID        `json:"id"`
	Code       string           `json:"code"`
	Status     enum.OrderStatus `json:"status"`
	Version    int              `json:"version"`
	Customer   DriverCustomer   `json:"customer"`
	Address    DriverAddress    `json:"address"`
	TotalCents money.Cents      `json:"totalCents"`
	ItemCount  int              `json:"itemCount"`
}

// DeliveryGroup is the driver's orders of one slot. Orders with no slot (courier)
// come last, in a group with a null slot.
type DeliveryGroup struct {
	Slot   *SlotRef      `json:"slot"`
	Orders []DriverOrder `json:"orders"`
}

// driverStatuses are the statuses a driver has work or a record in.
var driverStatuses = []string{string(enum.OrderStatusPaid), string(enum.OrderStatusPicking), string(enum.OrderStatusPacked),
	string(enum.OrderStatusOutForDelivery), string(enum.OrderStatusDeliveryFailed), string(enum.OrderStatusDelivered)}

// day reads YYYY-MM-DD as a Colombo date; empty is today.
func day(s string) (time.Time, error) {
	if s == "" {
		d := clock.Colombo(clock.Now())
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location()), nil
	}
	t, err := time.ParseInLocation(time.DateOnly, s, clock.Colombo(clock.Now()).Location())
	if err != nil {
		return t, apperr.Validation(map[string]string{"day": "invalid"})
	}
	return t, nil
}

// Deliveries lists the caller's own orders for a day, by slot. A slot order
// belongs to the day its window starts; a courier order to the day it was paid.
func (s *Service) Deliveries(ctx context.Context, scope db.Scope, dayText string) ([]DeliveryGroup, error) {
	from, err := day(dayText)
	if err != nil {
		return nil, err
	}
	out := []DeliveryGroup{}
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id, o.code, o.status, o.version, u.name, o.address, o.lat, o.lng, o.total_cents, sl.starts_at, sl.ends_at,
			       (SELECT count(*) FROM order_items i WHERE i.order_id = o.id AND i.revision_no = o.revision_no AND i.state IN ('ACTIVE', 'SUBSTITUTE'))
			FROM orders o JOIN users u ON u.id = o.customer_id LEFT JOIN slots sl ON sl.id = o.slot_id
			WHERE o.driver_id = $1 AND o.status = ANY($2) AND COALESCE(sl.starts_at, o.paid_at) >= $3 AND COALESCE(sl.starts_at, o.paid_at) < $4
			ORDER BY sl.starts_at NULLS LAST, o.paid_at, o.id`, scope.UserID, driverStatuses, from, from.AddDate(0, 0, 1))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d DriverOrder
			var snap addressSnapshot
			var starts, ends *time.Time
			if err := rows.Scan(&d.ID, &d.Code, &d.Status, &d.Version, &d.Customer.Name, &snap, &d.Address.Lat, &d.Address.Lng,
				&d.TotalCents, &starts, &ends, &d.ItemCount); err != nil {
				return err
			}
			d.Customer.Phone = snap.Phone
			d.Address.Label, d.Address.Line1, d.Address.Landmark = snap.Label, snap.Line1, snap.Landmark
			var slot *SlotRef
			if starts != nil && ends != nil {
				slot = &SlotRef{StartsAt: *starts, EndsAt: *ends}
			}
			if n := len(out); n > 0 && sameSlot(out[n-1].Slot, slot) {
				out[n-1].Orders = append(out[n-1].Orders, d)
			} else {
				out = append(out, DeliveryGroup{Slot: slot, Orders: []DriverOrder{d}})
			}
		}
		return rows.Err()
	})
	return out, err
}

func sameSlot(a, b *SlotRef) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.StartsAt.Equal(b.StartsAt) && a.EndsAt.Equal(b.EndsAt)
}

// DriverStats is a driver's record: counts only, no money (docs/06 section 9).
type DriverStats struct {
	Delivered int `json:"delivered"`
	Failed    int `json:"failed"`
}

// Stats counts the caller's deliveries and failed tries between two Colombo
// dates, both days included. Without dates it is the last 30 days.
func (s *Service) Stats(ctx context.Context, scope db.Scope, fromText, toText string) (DriverStats, error) {
	to, err := day(toText)
	if err != nil {
		return DriverStats{}, err
	}
	from := to.AddDate(0, 0, -29)
	if fromText != "" {
		if from, err = day(fromText); err != nil {
			return DriverStats{}, err
		}
	}
	if from.After(to) {
		return DriverStats{}, apperr.Validation(map[string]string{"from": "after_to"})
	}
	var out DriverStats
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE to_status = 'DELIVERED'), count(*) FILTER (WHERE to_status = 'DELIVERY_FAILED')
			FROM order_events
			WHERE actor_id = $1 AND to_status IN ('DELIVERED', 'DELIVERY_FAILED') AND at >= $2 AND at < $3`,
			scope.UserID, from, to.AddDate(0, 0, 1)).Scan(&out.Delivered, &out.Failed)
	})
	return out, err
}
