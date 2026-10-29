// Package catalog is the master product list and each shop's slice of it. This
// file set is the admin and console side; the public reads come with P14.
package catalog

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
	"gp/internal/shared/optional"
)

// Category is one node of the two-level tree: a top level or its child.
type Category struct {
	ID             uuid.UUID  `json:"id"`
	ParentID       *uuid.UUID `json:"parentId"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	Sort           int        `json:"sort"`
	DefaultWeightG int        `json:"defaultWeightG"`
}

// CategoryInput is a create or PATCH body. A create needs slug and name; the
// rest of the fields fall back to the defaults.
type CategoryInput struct {
	Slug           *string                   `json:"slug"`
	Name           *string                   `json:"name"`
	ParentID       optional.Field[uuid.UUID] `json:"parentId"`
	Sort           *int                      `json:"sort"`
	DefaultWeightG *int                      `json:"defaultWeightG"`
}

// Product is one master catalog entry, shared by every shop that sells it.
type Product struct {
	ID         uuid.UUID          `json:"id"`
	Barcode    *string            `json:"barcode"`
	Name       string             `json:"name"`
	Brand      *string            `json:"brand"`
	SizeLabel  *string            `json:"sizeLabel"`
	Unit       enum.Unit          `json:"unit"`
	WeightG    *int               `json:"weightG"`
	CategoryID *uuid.UUID         `json:"categoryId"`
	ImageURL   *string            `json:"imageUrl"`
	Keywords   string             `json:"keywords"`
	Status     enum.ProductStatus `json:"status"`
}

// ProductInput is a create or PATCH body. A create needs only the name.
type ProductInput struct {
	Barcode    optional.Field[string]    `json:"barcode"`
	Name       *string                   `json:"name"`
	Brand      optional.Field[string]    `json:"brand"`
	SizeLabel  optional.Field[string]    `json:"sizeLabel"`
	Unit       *enum.Unit                `json:"unit"`
	WeightG    optional.Field[int]       `json:"weightG"`
	CategoryID optional.Field[uuid.UUID] `json:"categoryId"`
	ImageURL   optional.Field[string]    `json:"imageUrl"`
	Keywords   *string                   `json:"keywords"`
	Status     *enum.ProductStatus       `json:"status"`
}

// ShopProduct is a product as one shop sells it. Prices are cents. StockQty is
// a plain JSON number, or null when the shop does not track stock.
type ShopProduct struct {
	ID               uuid.UUID      `json:"id"`
	ProductID        uuid.UUID      `json:"productId"`
	Name             string         `json:"name"`
	Brand            *string        `json:"brand"`
	SizeLabel        *string        `json:"sizeLabel"`
	Barcode          *string        `json:"barcode"`
	ImageURL         *string        `json:"imageUrl"`
	ShopPriceCents   int64          `json:"shopPriceCents"`
	MRPCents         *int64         `json:"mrpCents"`
	OnlinePriceCents int64          `json:"onlinePriceCents"`
	StockQty         *json.Number   `json:"stockQty"`
	IsAvailable      bool           `json:"isAvailable"`
	MaxPerOrder      int            `json:"maxPerOrder"`
	ManagedBy        enum.ManagedBy `json:"managedBy"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// ShopProductInput links a master product to a shop (admin).
type ShopProductInput struct {
	ProductID      uuid.UUID `json:"productId"`
	ShopPriceCents int64     `json:"shopPriceCents"`
	MRPCents       *int64    `json:"mrpCents"`
	StockQty       *int64    `json:"stockQty"`
	IsAvailable    *bool     `json:"isAvailable"`
	MaxPerOrder    *int      `json:"maxPerOrder"`
}

// ShopProductPatch is the console PATCH body. Fields the shop database owns
// (everything but maxPerOrder) are refused for connector-managed products.
type ShopProductPatch struct {
	ShopPriceCents *int64                `json:"shopPriceCents"`
	MRPCents       optional.Field[int64] `json:"mrpCents"`
	StockQty       optional.Field[int64] `json:"stockQty"`
	IsAvailable    *bool                 `json:"isAvailable"`
	MaxPerOrder    *int                  `json:"maxPerOrder"`
}

// Page is a slice of a list. NextCursor is empty on the last page.
type Page[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"nextCursor"`
}
