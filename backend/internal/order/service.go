package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/cartclient"
	"gp/internal/delivery"
	"gp/internal/shared/db"
	"gp/internal/shop"
	"gp/internal/user"
)

// What the order module needs from the others. Each is the service of that
// module, never its repo (CLAUDE.md section 3).

type Shops interface {
	OrderFacts(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time) (shop.OrderFacts, error)
}

type Delivery interface {
	RuleFor(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (delivery.RuleRow, error)
	OpenSlots(ctx context.Context, tx *db.Tx, shopID uuid.UUID, now time.Time) ([]delivery.PublicSlot, error)
	CheckSlot(ctx context.Context, tx *db.Tx, shopID, slotID uuid.UUID, now time.Time) error
}

type Users interface {
	AddressFor(ctx context.Context, tx *db.Tx, userID, id uuid.UUID) (user.Address, error)
	PhoneVerified(ctx context.Context, tx *db.Tx, userID uuid.UUID) (bool, error)
}

type Products interface {
	BillProducts(ctx context.Context, tx *db.Tx, shopID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]billing.ProductSnapshot, error)
}

// Carts reads a shopper's cart from the cart service. *cartclient.Client is the real one.
type Carts interface {
	GetCart(ctx context.Context, tenantID, cartID string) (cartclient.Cart, error)
}

type Service struct {
	db       *db.DB
	shops    Shops
	delivery Delivery
	users    Users
	products Products
	carts    Carts
}

func NewService(d *db.DB, shops Shops, dl Delivery, users Users, products Products, carts Carts) *Service {
	return &Service{db: d, shops: shops, delivery: dl, users: users, products: products, carts: carts}
}
