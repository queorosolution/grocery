package catalog

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shop"
)

const dealsOnHome = 12

// Home is the first page: the shops that deliver to the pin (all active shops
// when there is none), the best deals among them, and promos. The pin is
// rounded to 2 decimals (about 1 km) before anything is measured, so the exact
// place is never used and one cached answer fits a whole neighbourhood.
func (s *Service) Home(ctx context.Context, pin *geo.Point) (Home, error) {
	pin = roundedPin(pin)
	now := clock.Now()
	out := Home{Shops: []ShopCard{}, Deals: []Deal{}, Promos: []Promo{}}
	err := s.db.Tx(ctx, db.Public(), func(ctx context.Context, tx *db.Tx) error {
		shops, err := s.visibleShops(ctx, tx, pin, now)
		if err != nil || len(shops) == 0 {
			return err
		}
		next, err := s.delivery.NextSlots(ctx, tx, now)
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, len(shops))
		byID := make(map[uuid.UUID]ShopCard, len(shops))
		for i := range shops {
			if at, ok := next[shops[i].ID]; ok {
				shops[i].NextSlot = &at
			}
			ids[i], byID[shops[i].ID] = shops[i].ID, shops[i]
		}
		out.Shops = shops
		deals, err := dealsOf(ctx, tx, ids, dealsOnHome)
		for _, d := range deals {
			d.ShopSlug, d.ShopName = byID[d.shopID].Slug, byID[d.shopID].Name
			out.Deals = append(out.Deals, d.Deal)
		}
		return err
	})
	return out, err
}

// visibleShops are the shops a visitor can shop from: those that deliver to the
// pin, nearest first, or every active shop by name when there is no pin.
func (s *Service) visibleShops(ctx context.Context, tx *db.Tx, pin *geo.Point, now time.Time) ([]ShopCard, error) {
	cards, err := s.shops.Cards(ctx, tx, now)
	if err != nil {
		return nil, err
	}
	out := make([]ShopCard, 0, len(cards))
	if pin == nil {
		for _, c := range cards {
			out = append(out, cardOf(c))
		}
		return out, nil
	}
	reach, err := s.delivery.DeliveringShops(ctx, tx, *pin)
	if err != nil {
		return nil, err
	}
	bySlug := make(map[string]int, len(cards))
	for i, c := range cards {
		bySlug[c.Slug] = i
	}
	for _, r := range reach {
		if i, ok := bySlug[r.Slug]; ok {
			card, km := cardOf(cards[i]), r.DistanceKm
			card.DistanceKm = &km
			out = append(out, card)
		}
	}
	return out, nil
}

// roundedPin rounds a pin to 2 decimals.
func roundedPin(p *geo.Point) *geo.Point {
	if p == nil {
		return nil
	}
	return &geo.Point{Lat: math.Round(p.Lat*100) / 100, Lng: math.Round(p.Lng*100) / 100}
}

func cardOf(c shop.Card) ShopCard {
	return ShopCard{ID: c.ID, Slug: c.Slug, Name: c.Name, LogoURL: c.LogoURL, ThemeColor: c.ThemeColor, OpenNow: c.OpenNow}
}
