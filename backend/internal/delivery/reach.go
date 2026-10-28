package delivery

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/geo"
)

// Reachable is a shop that delivers to a pin. DistanceKm is the straight line.
type Reachable struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	DistanceKm Dec2   `json:"distanceKm"`
}

// DeliveringShops lists the active shops whose delivery distance covers the pin,
// nearest first (FR-ADDR-02). A shop with no fee rule at all (its own or the
// default) cannot price a delivery, so it does not appear.
func (s *Service) DeliveringShops(ctx context.Context, tx *db.Tx, p geo.Point) ([]Reachable, error) {
	shops, err := s.shops.ActiveLocations(ctx, tx)
	if err != nil {
		return nil, err
	}
	rules, err := listRules(ctx, tx)
	if err != nil {
		return nil, err
	}
	var platform *Rule
	own := map[uuid.UUID]Rule{}
	for _, r := range rules {
		if r.ShopID == nil {
			rule := r.Rule
			platform = &rule
		} else {
			own[*r.ShopID] = r.Rule
		}
	}
	out := []Reachable{}
	for _, sh := range shops {
		rule, ok := own[sh.ID]
		if !ok {
			if platform == nil {
				continue
			}
			rule = *platform
		}
		m := geo.HaversineM(geo.Point{Lat: sh.Lat, Lng: sh.Lng}, p)
		if rule.InRange(m) {
			out = append(out, Reachable{Slug: sh.Slug, Name: sh.Name, DistanceKm: Dec2((m + 5) / 10)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DistanceKm != out[j].DistanceKm {
			return out[i].DistanceKm < out[j].DistanceKm
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}
