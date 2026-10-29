package order

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/cartclient"
	"gp/internal/delivery"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shared/money"
	"gp/internal/shop"
)

// basket is the cart read through our own ids: one line per shop product, in cart order.
type basket struct {
	ids  []uuid.UUID
	qty  map[uuid.UUID]int
	seen map[uuid.UUID]money.Cents // the price the cart showed
}

// Quote prices the customer's cart as it would be billed now (FR-BILL-01).
// It writes nothing. Quantities come from the cart service, prices from our
// database, and the shop, the address and the slot are checked against the caller.
func (s *Service) Quote(ctx context.Context, scope db.Scope, in QuoteInput) (Quote, error) {
	if err := in.validate(); err != nil {
		return Quote{}, err
	}
	now := clock.Now()

	// The address and the phone are the caller's own data, so they are read in the caller's scope.
	var addr geo.Point
	var phoneOK bool
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		a, err := s.users.AddressFor(ctx, tx, scope.UserID, in.AddressID)
		if err != nil {
			return err
		}
		addr = geo.Point{Lat: a.Lat, Lng: a.Lng}
		phoneOK, err = s.users.PhoneVerified(ctx, tx, scope.UserID)
		return err
	})
	if err != nil {
		return Quote{}, err
	}

	// Shop facts include payment readiness, which only the system scope may read.
	// Nothing of the caller is read there.
	var (
		facts shop.OrderFacts
		rule  delivery.RuleRow
		slots []delivery.PublicSlot
	)
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		var err error
		if facts, err = s.shops.OrderFacts(ctx, tx, in.ShopID, now); err != nil {
			return err
		}
		if rule, err = s.delivery.RuleFor(ctx, tx, in.ShopID); err != nil {
			return err
		}
		if slots, err = s.delivery.OpenSlots(ctx, tx, in.ShopID, now); err != nil {
			return err
		}
		if in.SlotID != nil {
			return s.delivery.CheckSlot(ctx, tx, in.ShopID, *in.SlotID, now)
		}
		return nil
	})
	if err != nil {
		return Quote{}, err
	}

	cart, err := s.cart(ctx, facts, in.CartID)
	if err != nil {
		return Quote{}, err
	}
	b := basketOf(cart)
	lines, err := linesOf(b, in.Lines)
	if err != nil {
		return Quote{}, err
	}

	var products map[uuid.UUID]billing.ProductSnapshot
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		products, err = s.products.BillProducts(ctx, tx, in.ShopID, b.ids)
		return err
	})
	if err != nil {
		return Quote{}, err
	}

	source := "DEFAULT"
	if rule.ShopID != nil {
		source = "SHOP"
	}
	bill, err := billing.Build(billing.BillInput{
		Shop: billing.ShopSnapshot{
			Location: facts.Location, MinOrderCents: money.Cents(facts.MinOrderCents),
			MinPlatformMarginCents: money.Cents(facts.MinPlatformMarginCents), MaxLines: facts.MaxLines, Open: facts.Open,
		},
		Lines: lines, Products: products, Address: addr, FeeRule: rule.Rule, FeeRuleSource: source,
		PhoneVerified: phoneOK, Now: now,
	})
	if err != nil {
		return Quote{}, err
	}
	return Quote{Bill: bill, Slots: slots}, nil
}

// cart reads the cart from the shop's own cart tenant. A cart that is not in
// that tenant is the cart of another shop: the cart service cannot tell the
// two apart, and neither should the caller.
func (s *Service) cart(ctx context.Context, facts shop.OrderFacts, cartID string) (cartclient.Cart, error) {
	if facts.CartTenant == "" {
		return cartclient.Cart{}, apperr.New(apperr.CartUnavailable)
	}
	cart, err := s.carts.GetCart(ctx, facts.CartTenant, cartID)
	switch {
	case errors.Is(err, cartclient.ErrNotFound):
		return cartclient.Cart{}, apperr.New(apperr.CartWrongShop)
	case err != nil:
		return cartclient.Cart{}, cartclient.ToAppError(err)
	case cart.Status != "open" || len(cart.Lines) == 0:
		return cartclient.Cart{}, apperr.New(apperr.CartEmpty)
	}
	return cart, nil
}

// basketOf keeps the cart lines that name one of our products. A line the cart
// holds without an item, or with an id that is not ours, cannot be sold. The
// same product on two lines is added together.
func basketOf(c cartclient.Cart) basket {
	b := basket{qty: map[uuid.UUID]int{}, seen: map[uuid.UUID]money.Cents{}}
	for _, l := range c.Lines {
		if l.ItemID == nil {
			continue
		}
		id, err := uuid.Parse(*l.ItemID)
		if err != nil {
			continue
		}
		if _, ok := b.qty[id]; !ok {
			b.ids = append(b.ids, id)
			b.seen[id] = money.Cents(l.UnitPrice)
		}
		b.qty[id] += l.Quantity
	}
	return b
}

// linesOf joins the cart with the customer's substitution choices. A choice for
// something that is not in the cart is a bad request.
func linesOf(b basket, refs []QuoteRef) ([]billing.LineInput, error) {
	pref := map[uuid.UUID]QuoteRef{}
	for i, r := range refs {
		if _, ok := b.qty[r.ItemID]; !ok {
			return nil, apperr.Validation(map[string]string{"lines." + strconv.Itoa(i) + ".itemId": "not_in_cart"})
		}
		pref[r.ItemID] = r
	}
	out := make([]billing.LineInput, 0, len(b.ids))
	for _, id := range b.ids {
		shown := b.seen[id]
		out = append(out, billing.LineInput{ShopProductID: id, Qty: b.qty[id], SubPref: pref[id].SubPref, CartUnitPrice: &shown})
	}
	return out, nil
}
