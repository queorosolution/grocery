package catalog

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/internal/pricing"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shop"
)

// Shops is what the catalog needs from the shop module.
type Shops interface {
	Exists(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error)
	IDBySlug(ctx context.Context, tx *db.Tx, slug string) (uuid.UUID, bool, error)
	Cards(ctx context.Context, tx *db.Tx, now time.Time) ([]shop.Card, error)
	PageBySlug(ctx context.Context, tx *db.Tx, slug string, now time.Time) (shop.PublicPage, bool, error)
}

// Delivery is what the public pages need from the delivery module.
type Delivery interface {
	DeliveringShops(ctx context.Context, tx *db.Tx, p geo.Point) ([]delivery.Reachable, error)
	NextSlots(ctx context.Context, tx *db.Tx, now time.Time) (map[uuid.UUID]time.Time, error)
}

type Service struct {
	db       *db.DB
	shops    Shops
	delivery Delivery
	pricing  *pricing.Service
	imgBase  string // IMG_BASE_URL: the only place product images may live
}

func NewService(d *db.DB, shops Shops, dl Delivery, p *pricing.Service, imgBaseURL string) *Service {
	return &Service{db: d, shops: shops, delivery: dl, pricing: p, imgBase: imgBaseURL}
}

// blankToNil turns "no text" into "no value", as the database stores it.
func blankToNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
