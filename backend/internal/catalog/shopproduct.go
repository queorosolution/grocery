package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/events"
	"gp/internal/pricing"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
)

// Limits.
const (
	maxStock           = 1_000_000_000
	defaultMaxPerOrder = 20
)

// ShopProducts is the console list: the shop's own products, by name.
func (s *Service) ShopProducts(ctx context.Context, scope db.Scope, shopID uuid.UUID, q, cur string, available *bool) (Page[ShopProduct], error) {
	after, err := parseCursor(cur)
	if err != nil {
		return Page[ShopProduct]{}, err
	}
	if err := validate.New().Str("q", q, 0, maxSearch).Err(); err != nil {
		return Page[ShopProduct]{}, err
	}
	var rows []ShopProduct
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		rows, err = listShopProducts(ctx, tx, shopID, strings.TrimSpace(q), available, after)
		return err
	})
	return cut(rows, func(sp ShopProduct) fmt.Stringer { return cursor{Name: sp.Name, ID: sp.ID} }), err
}

// AddShopProduct puts a master product on sale in a shop by hand (MANUAL).
func (s *Service) AddShopProduct(ctx context.Context, scope db.Scope, shopID uuid.UUID, in ShopProductInput) (ShopProduct, error) {
	v := validate.New().Check("productId", in.ProductID != uuid.Nil, "invalid")
	checkPrice(v, "shopPriceCents", &in.ShopPriceCents)
	checkPrice(v, "mrpCents", in.MRPCents)
	checkStock(v, in.StockQty)
	n := pricing.NewShopProduct{
		ShopID: shopID, ProductID: in.ProductID, ShopPrice: money.Cents(in.ShopPriceCents), StockQty: in.StockQty,
		Available: in.IsAvailable == nil || *in.IsAvailable, MaxPerOrder: defaultMaxPerOrder,
		ManagedBy: enum.ManagedByManual, Source: enum.PriceChangeSourceManual,
	}
	if in.MRPCents != nil {
		mrp := money.Cents(*in.MRPCents)
		n.MRP = &mrp
	}
	if in.MaxPerOrder != nil {
		v.Int("maxPerOrder", int64(*in.MaxPerOrder), 1, 999)
		n.MaxPerOrder = *in.MaxPerOrder
	}
	if err := v.Err(); err != nil {
		return ShopProduct{}, err
	}
	var out ShopProduct
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		ok, err := s.shops.Exists(ctx, tx, shopID)
		if err != nil || !ok {
			return apperr.NotFoundUnless(err, ok)
		}
		id, _, err := s.pricing.AddShopProduct(ctx, tx, scope, n)
		if err != nil {
			if db.IsUniqueViolation(err) {
				return validate.New().Check("productId", false, "taken").Err()
			}
			return priceError(err, in.ShopPriceCents, in.MRPCents)
		}
		if out, _, err = shopProductByID(ctx, tx, shopID, id); err != nil {
			return err
		}
		return recordShopProduct(ctx, tx, scope, shopID, "shop_product.create", nil, &out)
	})
	return out, err
}

// UpdateShopProduct is the console edit. Whatever the shop database owns is
// refused for a connector-managed product; max per order is always ours.
func (s *Service) UpdateShopProduct(ctx context.Context, scope db.Scope, shopID, id uuid.UUID, in ShopProductPatch) (ShopProduct, error) {
	var out ShopProduct
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := shopProductByID(ctx, tx, shopID, id)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if err := checkPatch(before, in); err != nil {
			return err
		}
		priceChanged, err := s.applyPrices(ctx, tx, scope, shopID, before, in)
		if err != nil {
			return err
		}
		fieldsChanged, err := applyFields(ctx, tx, before, in)
		if err != nil {
			return err
		}
		if fieldsChanged && !priceChanged {
			if err := events.AddCatalogChanged(ctx, tx, shopID, []uuid.UUID{id}); err != nil {
				return err
			}
		}
		if out, _, err = shopProductByID(ctx, tx, shopID, id); err != nil {
			return err
		}
		return recordShopProduct(ctx, tx, scope, shopID, "shop_product.update", &before, &out)
	})
	return out, err
}

// checkPatch refuses connector-owned fields first, then checks the values.
func checkPatch(cur ShopProduct, in ShopProductPatch) error {
	v := validate.New()
	if cur.ManagedBy == enum.ManagedByConnector {
		v.Check("shopPriceCents", in.ShopPriceCents == nil, "managed_by_connector").
			Check("mrpCents", !in.MRPCents.Set, "managed_by_connector").
			Check("stockQty", !in.StockQty.Set, "managed_by_connector").
			Check("isAvailable", in.IsAvailable == nil, "managed_by_connector")
		if err := v.Err(); err != nil {
			return err
		}
	}
	checkPrice(v, "shopPriceCents", in.ShopPriceCents)
	checkPrice(v, "mrpCents", in.MRPCents.Ptr())
	checkStock(v, in.StockQty.Ptr())
	if in.MaxPerOrder != nil {
		v.Int("maxPerOrder", int64(*in.MaxPerOrder), 1, 999)
	}
	return v.Err()
}

// applyPrices sends a changed shop price or MRP to the pricing module.
func (s *Service) applyPrices(ctx context.Context, tx *db.Tx, scope db.Scope, shopID uuid.UUID, cur ShopProduct, in ShopProductPatch) (bool, error) {
	if in.ShopPriceCents == nil && !in.MRPCents.Set {
		return false, nil
	}
	price, mrp := cur.ShopPriceCents, cur.MRPCents
	if in.ShopPriceCents != nil {
		price = *in.ShopPriceCents
	}
	if in.MRPCents.Set {
		mrp = in.MRPCents.Ptr()
	}
	var m *money.Cents
	if mrp != nil {
		c := money.Cents(*mrp)
		m = &c
	}
	applied, err := s.pricing.SetShopPrice(ctx, tx, scope, shopID, cur.ID, money.Cents(price), m, enum.PriceChangeSourceManual)
	if err != nil {
		return false, priceError(err, price, mrp)
	}
	return applied.Changed, nil
}

// applyFields stores stock, availability and max per order. It writes only the
// fields the body carries, and reports whether any of them is different.
func applyFields(ctx context.Context, tx *db.Tx, cur ShopProduct, in ShopProductPatch) (bool, error) {
	changed := in.StockQty.Set ||
		(in.IsAvailable != nil && *in.IsAvailable != cur.IsAvailable) ||
		(in.MaxPerOrder != nil && *in.MaxPerOrder != cur.MaxPerOrder)
	if !changed {
		return false, nil
	}
	return true, saveShopProductFields(ctx, tx, cur.ID, in)
}

func checkPrice(v *validate.Validator, field string, cents *int64) {
	if cents != nil {
		v.Int(field, *cents, 1, pricing.MaxShopPriceCents)
	}
}

func checkStock(v *validate.Validator, qty *int64) {
	if qty != nil {
		v.Int("stockQty", *qty, 0, maxStock)
	}
}

// priceError names the field to blame when the prices leave no valid customer
// price: the MRP when it is under the shop price, else the shop price itself.
func priceError(err error, shopPrice int64, mrp *int64) error {
	if !errors.Is(err, pricing.ErrMRPBelowShopPrice) {
		return err
	}
	field := "shopPriceCents"
	if mrp != nil && *mrp < shopPrice {
		field = "mrpCents"
	}
	return validate.New().Check(field, false, "invalid_price").Err()
}

func recordShopProduct(ctx context.Context, tx *db.Tx, scope db.Scope, shopID uuid.UUID, action string, before, after *ShopProduct) error {
	e := audit.Entry{Action: action, Entity: "shop_product", EntityID: after.ID.String(), ShopID: shopID, After: after}
	if before != nil {
		e.Before = before
	}
	return audit.Write(ctx, tx, scope, e)
}
