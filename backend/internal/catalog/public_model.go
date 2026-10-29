package catalog

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/internal/shop"
)

// PublicPage is a page of a public list. NextCursor is null on the last page.
type PublicPage[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"nextCursor"`
}

// ShopCard is a shop on the home and search pages. DistanceKm is the straight
// line from the pin, null when there is no pin.
type ShopCard struct {
	ID         uuid.UUID      `json:"id"`
	Slug       string         `json:"slug"`
	Name       string         `json:"name"`
	LogoURL    *string        `json:"logoUrl"`
	ThemeColor string         `json:"themeColor"`
	OpenNow    bool           `json:"openNow"`
	NextSlot   *time.Time     `json:"nextSlot,omitempty"`
	DistanceKm *delivery.Dec2 `json:"distanceKm"`
}

// Deal is a product sold under its MRP.
type Deal struct {
	ShopProductID    uuid.UUID `json:"shopProductId"`
	ShopSlug         string    `json:"shopSlug"`
	ShopName         string    `json:"shopName"`
	Name             string    `json:"name"`
	ImageURL         *string   `json:"imageUrl"`
	OnlinePriceCents int64     `json:"onlinePriceCents"`
	MRPCents         int64     `json:"mrpCents"`
	SaveCents        int64     `json:"saveCents"`
}

// Promo is a banner. There is no promo data yet, so the list is always empty.
type Promo struct {
	ShopSlug string  `json:"shopSlug"`
	Title    string  `json:"title"`
	ImageURL *string `json:"imageUrl"`
}

type Home struct {
	Shops  []ShopCard `json:"shops"`
	Deals  []Deal     `json:"deals"`
	Promos []Promo    `json:"promos"`
}

// ProductCard is a product in a shop as a customer sees it. ShopSlug is set on
// lists that mix shops.
type ProductCard struct {
	ShopProductID    uuid.UUID `json:"shopProductId"`
	ProductID        uuid.UUID `json:"productId"`
	ShopSlug         string    `json:"shopSlug,omitempty"`
	Name             string    `json:"name"`
	Brand            *string   `json:"brand"`
	SizeLabel        *string   `json:"sizeLabel"`
	Unit             string    `json:"unit"`
	ImageURL         *string   `json:"imageUrl"`
	OnlinePriceCents int64     `json:"onlinePriceCents"`
	MRPCents         *int64    `json:"mrpCents"`
	Available        bool      `json:"available"`
	MaxPerOrder      int       `json:"maxPerOrder"`
}

type SearchResult struct {
	Shops    []ShopCard              `json:"shops"`
	Products PublicPage[ProductCard] `json:"products"`
}

// ShopCategory is a category with the number of products the shop sells in it
// (a top-level count includes its children).
type ShopCategory struct {
	ID       uuid.UUID  `json:"id"`
	ParentID *uuid.UUID `json:"parentId"`
	Slug     string     `json:"slug"`
	Name     string     `json:"name"`
	Count    int        `json:"count"`
}

// ShopPage is the shop page: the shop and what it sells, by category.
type ShopPage struct {
	shop.PublicPage
	Categories []ShopCategory `json:"categories"`
}

// CategoryNode is a category of the public tree.
type CategoryNode struct {
	ID       uuid.UUID      `json:"id"`
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Children []CategoryNode `json:"children"`
}
