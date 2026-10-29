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
	"gp/internal/shop"
	"gp/internal/user"
)

// What the order module needs from the others. Each is the service of that
// module, never its repo (CLAUDE.md section 3).

type Shops interface {
	OrderFacts(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time) (shop.OrderFacts, error)
	TimeoutsOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (shop.Timeouts, error)
	BillFacts(ctx context.Context, tx *db.Tx, id uuid.UUID) (shop.BillFacts, error)
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
}

func NewService(d *db.DB, shops Shops, dl Delivery, users Users, products Products, carts Carts, coupons Coupons, fin Finance) *Service {
	return &Service{db: d, shops: shops, delivery: dl, users: users, products: products, carts: carts, coupons: coupons, finance: fin}
}
