package pricing

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/logx"
	"gp/internal/shared/money"
)

// Shops is what pricing needs from the shop module: whether a shop exists and
// its default markup. The shop service implements it.
type Shops interface {
	Exists(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error)
	MarkupBP(ctx context.Context, tx *db.Tx, id uuid.UUID) (bp int, found bool, err error)
}

type Service struct {
	db    *db.DB
	shops Shops
}

func NewService(d *db.DB, shops Shops) *Service { return &Service{db: d, shops: shops} }

// Applied is what SetShopPrice stored.
type Applied struct {
	ShopPrice, Online money.Cents
	MRP               *money.Cents
	Changed           bool // a price or the MRP is different from before
}

// SetShopPrice stores the shop price and MRP of one shop product and works out
// the customer price from them. It runs in the caller's transaction and, when
// something changed, writes the price_changes row and the catalog.changed
// event in it. A product of another shop is NOT_FOUND.
//
// ErrMRPBelowShopPrice comes back as is: the caller knows which field to blame.
func (s *Service) SetShopPrice(ctx context.Context, tx *db.Tx, scope db.Scope, shopID, shopProductID uuid.UUID,
	shopPrice money.Cents, mrp *money.Cents, source enum.PriceChangeSource) (Applied, error) {
	cur, found, err := lockRow(ctx, tx, shopID, shopProductID)
	if err != nil {
		return Applied{}, err
	}
	if !found {
		return Applied{}, apperr.New(apperr.NotFound)
	}
	bp, err := s.markupFor(ctx, tx, shopID, cur.categoryID)
	if err != nil {
		return Applied{}, err
	}
	online, err := OnlinePrice(shopPrice, mrp, bp)
	if err != nil {
		return Applied{}, err
	}
	out := Applied{ShopPrice: shopPrice, Online: online, MRP: mrp,
		Changed: shopPrice != cur.shopPrice || online != cur.online || !sameMRP(mrp, cur.mrp)}
	if !out.Changed {
		return out, nil
	}
	if err := storePrices(ctx, tx, shopProductID, shopPrice, mrp, online); err != nil {
		return Applied{}, err
	}
	if shopPrice != cur.shopPrice || online != cur.online {
		if err := logChanges(ctx, tx, scope, source, []priceChange{{
			id: shopProductID, oldShop: cur.shopPrice, newShop: shopPrice, oldOnline: cur.online, newOnline: online,
		}}); err != nil {
			return Applied{}, err
		}
	}
	return out, events.AddCatalogChanged(ctx, tx, shopID, []uuid.UUID{shopProductID})
}

// Reprice works out every customer price of a shop again, after its markup or a
// category markup changed. Products whose prices leave no valid customer price
// keep the old one and are logged; the rest are updated, each with a
// price_changes row (source MARKUP) and one catalog.changed event per 200.
func (s *Service) Reprice(ctx context.Context, tx *db.Tx, scope db.Scope, shopID uuid.UUID) error {
	def, found, err := s.shops.MarkupBP(ctx, tx, shopID)
	if err != nil {
		return err
	}
	if !found {
		return apperr.New(apperr.NotFound)
	}
	rows, err := lockShopRows(ctx, tx, shopID, def)
	if err != nil {
		return err
	}
	var changes []priceChange
	skipped := 0
	for _, r := range rows {
		online, err := OnlinePrice(r.shopPrice, r.mrp, r.markupBP)
		switch {
		case errors.Is(err, ErrMRPBelowShopPrice):
			skipped++
		case err != nil:
			return err
		case online != r.online:
			changes = append(changes, priceChange{id: r.id, oldShop: r.shopPrice, newShop: r.shopPrice, oldOnline: r.online, newOnline: online})
		}
	}
	if skipped > 0 {
		logx.From(ctx).Warn("reprice: products without a valid customer price kept their old price", "shopId", shopID, "skipped", skipped)
	}
	if len(changes) == 0 {
		return nil
	}
	if err := storeOnline(ctx, tx, changes); err != nil {
		return err
	}
	if err := logChanges(ctx, tx, scope, enum.PriceChangeSourceMarkup, changes); err != nil {
		return err
	}
	changed := make([]uuid.UUID, len(changes))
	for i, c := range changes {
		changed[i] = c.id
	}
	return events.AddCatalogChanged(ctx, tx, shopID, changed)
}

// markupFor is the category markup of the shop, else its default.
func (s *Service) markupFor(ctx context.Context, tx *db.Tx, shopID uuid.UUID, category *uuid.UUID) (int, error) {
	if category != nil {
		bp, found, err := categoryMarkup(ctx, tx, shopID, *category)
		if err != nil || found {
			return bp, err
		}
	}
	bp, found, err := s.shops.MarkupBP(ctx, tx, shopID)
	if err == nil && !found {
		err = apperr.New(apperr.NotFound)
	}
	return bp, err
}

func sameMRP(a, b *money.Cents) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
