package shop

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Card is an active shop as the public pages list it. Lat and Lng are for
// measuring distances on the server; they are not sent to the browser.
type Card struct {
	ID         uuid.UUID
	Slug       string
	Name       string
	LogoURL    *string
	ThemeColor string
	Lat, Lng   float64
	OpenNow    bool
}

// PublicPage is the shop page without its categories. Nothing private is here:
// the cart public key (pk_) is meant for browsers.
type PublicPage struct {
	ID              uuid.UUID         `json:"id"`
	Slug            string            `json:"slug"`
	Name            string            `json:"name"`
	LogoURL         *string           `json:"logoUrl"`
	ThemeColor      string            `json:"themeColor"`
	OpenNow         bool              `json:"openNow"`
	AcceptingOrders bool              `json:"acceptingOrders"`
	HoursToday      *HoursToday       `json:"hoursToday"`
	DeliveryType    enum.DeliveryType `json:"deliveryType"`
	MinOrderCents   int64             `json:"minOrderCents"`
	Cart            *CartKeys         `json:"cart"`
}

// HoursToday is today's opening time in Colombo, or the shop page shows null
// for a day the shop is closed.
type HoursToday struct {
	Opens  string `json:"opens"`
	Closes string `json:"closes"`
}

// CartKeys is what the browser needs to talk to the cart service.
type CartKeys struct {
	TenantID  string `json:"tenantId"`
	PublicKey string `json:"publicKey"`
}

// Cards lists the active shops by name, each with whether it is open at now.
func (s *Service) Cards(ctx context.Context, tx *db.Tx, now time.Time) ([]Card, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, slug, name, logo_url, theme_color, lat, lng FROM shops WHERE status = 'ACTIVE' ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Card{}
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.LogoURL, &c.ThemeColor, &c.Lat, &c.Lng); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	weeks, err := activeWeeks(ctx, tx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].OpenNow = weeks[out[i].ID].openAt(now)
	}
	return out, nil
}

// PageBySlug is the public page of an ACTIVE shop; suspended and draft shops
// are not found.
func (s *Service) PageBySlug(ctx context.Context, tx *db.Tx, slug string, now time.Time) (PublicPage, bool, error) {
	var p PublicPage
	var tenant, key *string
	err := tx.QueryRow(ctx, `
		SELECT sh.id, sh.slug, sh.name, sh.logo_url, sh.theme_color, sh.accepting_orders,
		       ss.delivery_type, ss.min_order_cents, ss.cart_tenant_id, ss.cart_public_key
		FROM shops sh JOIN shop_settings ss ON ss.shop_id = sh.id
		WHERE sh.slug = $1 AND sh.status = 'ACTIVE'`, slug).
		Scan(&p.ID, &p.Slug, &p.Name, &p.LogoURL, &p.ThemeColor, &p.AcceptingOrders, &p.DeliveryType, &p.MinOrderCents, &tenant, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicPage{}, false, nil
	}
	if err != nil {
		return PublicPage{}, false, err
	}
	if tenant != nil && key != nil {
		p.Cart = &CartKeys{TenantID: *tenant, PublicKey: *key}
	}
	days, err := hoursOf(ctx, tx, p.ID)
	if err != nil {
		return PublicPage{}, false, err
	}
	week := weekOf(days)
	p.OpenNow = week.openAt(now)
	if sp, ok := week[isoWeekday(clock.Colombo(now).Weekday())]; ok {
		p.HoursToday = &HoursToday{Opens: formatClock(sp.opens), Closes: formatClock(sp.closes)}
	}
	return p, true, nil
}

// activeWeeks reads the opening hours of every active shop in one query.
func activeWeeks(ctx context.Context, tx *db.Tx) (map[uuid.UUID]weekHours, error) {
	rows, err := tx.Query(ctx, `
		SELECT h.shop_id, h.weekday, (EXTRACT(EPOCH FROM h.opens) / 60)::int, (EXTRACT(EPOCH FROM h.closes) / 60)::int
		FROM shop_hours h JOIN shops s ON s.id = h.shop_id WHERE s.status = 'ACTIVE'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]weekHours{}
	for rows.Next() {
		var id uuid.UUID
		var wd, opens, closes int
		if err := rows.Scan(&id, &wd, &opens, &closes); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = weekHours{}
		}
		out[id][wd] = span{opens: opens, closes: closes}
	}
	return out, rows.Err()
}
