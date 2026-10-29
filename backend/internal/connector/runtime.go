package connector

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/catalog"
	"gp/internal/pricing"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// What a run needs from other modules. They are interfaces so this package
// calls services, never their tables, and tests can swap the AWS parts.

// Pricer is the pricing service: the only place that works out a customer price.
type Pricer interface {
	SetShopPrice(ctx context.Context, tx *db.Tx, scope db.Scope, shopID, shopProductID uuid.UUID,
		shopPrice money.Cents, mrp *money.Cents, source enum.PriceChangeSource) (pricing.Applied, error)
	AddShopProduct(ctx context.Context, tx *db.Tx, scope db.Scope, n pricing.NewShopProduct) (uuid.UUID, pricing.Applied, error)
}

// Products adds master products (auto-create and the CREATE action).
type Products interface {
	CreateProductIn(ctx context.Context, tx *db.Tx, scope db.Scope, in catalog.ProductInput) (catalog.Product, error)
}

// Hours says whether a shop is open; scheduled runs only happen then.
type Hours interface {
	OpenAt(ctx context.Context, tx *db.Tx, shopID uuid.UUID, t time.Time) (bool, error)
}

// Schedules keeps one recurring schedule per active connector (sync-<id>).
// Both calls can be repeated safely; deleting a schedule that is gone is fine.
type Schedules interface {
	Upsert(ctx context.Context, connectorID uuid.UUID, intervalMin int) error
	Delete(ctx context.Context, connectorID uuid.UUID) error
}

// Invoker starts a run in the sync function without waiting for it (an async
// Lambda invoke). Nil means "run it here", which local runs and tests use.
type Invoker interface {
	Invoke(ctx context.Context, job Job) error
}

// Runtime is everything Run, the schedule and the issue actions need besides
// the database and the secret store.
type Runtime struct {
	Pricer    Pricer
	Products  Products
	Hours     Hours
	Schedules Schedules // nil: no schedule is kept (local runs)
	Invoker   Invoker
}

// Attach sets the runtime. It is called once at start-up.
func (s *Service) Attach(r Runtime) *Service {
	s.rt = r
	return s
}

// Job is what the sync function receives: from the schedule, from "Sync now"
// and from the trigger URL. ExternalIDs is set only by the trigger.
type Job struct {
	ConnectorID uuid.UUID       `json:"connectorId"`
	Trigger     enum.RunTrigger `json:"trigger"`
	ExternalIDs []string        `json:"externalIds,omitempty"`
}
