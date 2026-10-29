package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/cartclient"
	"gp/internal/coupon"
	"gp/internal/delivery"
	"gp/internal/finance"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/secrets"
	"gp/internal/shop"
	"gp/internal/team"
	"gp/internal/user"
)

// What the order module needs from the others. Each is the service of that
// module, never its repo (CLAUDE.md section 3).

type Shops interface {
	OrderFacts(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time) (shop.OrderFacts, error)
	TimeoutsOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (shop.Timeouts, error)
	BillFacts(ctx context.Context, tx *db.Tx, id uuid.UUID) (shop.BillFacts, error)
	DriverSourceOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (enum.DriverSource, error)
}

type Delivery interface {
	RuleFor(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (delivery.RuleRow, error)
	OpenSlots(ctx context.Context, tx *db.Tx, shopID uuid.UUID, now time.Time) ([]delivery.PublicSlot, error)
	CheckSlot(ctx context.Context, tx *db.Tx, shopID, slotID uuid.UUID, now time.Time) error
	BookSlot(ctx context.Context, tx *db.Tx, shopID, slotID uuid.UUID, now time.Time) error
	ReleaseSlot(ctx context.Context, tx *db.Tx, slotID uuid.UUID) error
}

type Users interface {
	AddressFor(ctx context.Context, tx *db.Tx, userID, id uuid.UUID) (user.Address, error)
	ContactOf(ctx context.Context, tx *db.Tx, userID uuid.UUID) (user.Contact, error)
}

type Products interface {
	BillProducts(ctx context.Context, tx *db.Tx, shopID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]billing.ProductSnapshot, error)
}

// Carts reads and deletes a shopper's cart in the cart service. *cartclient.Client is the real one.
type Carts interface {
	GetCart(ctx context.Context, tenantID, cartID string) (cartclient.Cart, error)
	DeleteCart(ctx context.Context, tenantID, cartID string) error
}

// Coupons reserves and gives back the uses of a coupon for an order, in the
// order's transaction. *coupon.Service is the real one.
type Coupons interface {
	Apply(ctx context.Context, tx *db.Tx, in coupon.ApplyInput, price coupon.Pricer) (coupon.Applied, error)
	Release(ctx context.Context, tx *db.Tx, orderID uuid.UUID) (bool, error)
	Confirm(ctx context.Context, tx *db.Tx, orderID uuid.UUID) error
}

// Finance posts the ledger rows of a paid order. *finance.Service is the real one.
type Finance interface {
	PostPaid(ctx context.Context, tx *db.Tx, f finance.PaidFacts) error
	PostDelivered(ctx context.Context, tx *db.Tx, f finance.DeliveredFacts) error
}

// Drivers answers who may deliver. *team.Service is the real one.
type Drivers interface {
	DriverKindOf(ctx context.Context, tx *db.Tx, userID, shopID uuid.UUID) (team.DriverKind, error)
	ShopDrivers(ctx context.Context, tx *db.Tx, shopID uuid.UUID) ([]team.Driver, error)
}

// Deps is everything the order service is built from.
type Deps struct {
	DB       *db.DB
	Shops    Shops
	Delivery Delivery
	Users    Users
	Products Products
	Carts    Carts
	Coupons  Coupons
	Finance  Finance
	Drivers  Drivers
	// Secrets gives the key that seals delivery codes. Nil only where no code is
	// ever made or read (the timer function).
	Secrets secrets.Getter
}

type Service struct {
	db       *db.DB
	shops    Shops
	delivery Delivery
	users    Users
	products Products
	carts    Carts
	coupons  Coupons
	finance  Finance
	drivers  Drivers
	secrets  secrets.Getter
}

func NewService(d Deps) *Service {
	return &Service{db: d.DB, shops: d.Shops, delivery: d.Delivery, users: d.Users, products: d.Products, carts: d.Carts,
		coupons: d.Coupons, finance: d.Finance, drivers: d.Drivers, secrets: d.Secrets}
}
