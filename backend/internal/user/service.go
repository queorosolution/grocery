package user

import (
	"context"

	"gp/internal/delivery"
	"gp/internal/notify"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shared/secrets"
	"gp/internal/shared/turnstile"
)

// Reach finds the shops that deliver to a pin. delivery.Service is the real one.
type Reach interface {
	DeliveringShops(ctx context.Context, tx *db.Tx, p geo.Point) ([]delivery.Reachable, error)
}

type Service struct {
	db        *db.DB
	secrets   secrets.Getter
	turnstile turnstile.Verifier
	notify    notify.Sender
	reach     Reach
	platform  string // PLATFORM_NAME, shown in the SMS text
}

func NewService(d *db.DB, sec secrets.Getter, ts turnstile.Verifier, n notify.Sender, reach Reach, platform string) *Service {
	return &Service{db: d, secrets: sec, turnstile: ts, notify: n, reach: reach, platform: platform}
}
