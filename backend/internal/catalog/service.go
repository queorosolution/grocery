package catalog

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"gp/internal/pricing"
	"gp/internal/shared/db"
)

// Shops is what the catalog needs from the shop module.
type Shops interface {
	Exists(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error)
}

type Service struct {
	db      *db.DB
	shops   Shops
	pricing *pricing.Service
	imgBase string // IMG_BASE_URL: the only place product images may live
}

func NewService(d *db.DB, shops Shops, p *pricing.Service, imgBaseURL string) *Service {
	return &Service{db: d, shops: shops, pricing: p, imgBase: imgBaseURL}
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
