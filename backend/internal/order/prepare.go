package order

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/cartclient"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shared/money"
	"gp/internal/shop"
	"gp/internal/user"
)

// prepared is what is known before the pricing transaction: the shop and the cart.
type prepared struct {
	shopID uuid.UUID
	facts  shop.OrderFacts
	basket basket
	lines  []billing.LineInput
}

// basket is the cart read through our own ids: one line per shop product, in cart order.
type basket struct {
	ids  []uuid.UUID
	qty  map[uuid.UUID]int
	seen map[uuid.UUID]money.Cents // the price the cart showed
}

// priced is a bill with the facts it was worked out from.
type priced struct {
	billing.Bill
	address user.Address
	contact user.Contact
}

// prepare reads the shop and then the cart. It is the same for a quote and for a
// submit, so both see the cart the same way. Shop facts include payment
// readiness, which only the system scope may read; nothing of the caller is
// read there.
func (s *Service) prepare(ctx context.Context, shopID uuid.UUID, cartID string, refs []QuoteRef) (prepared, error) {
	p := prepared{shopID: shopID}
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		p.facts, err = s.shops.OrderFacts(ctx, tx, shopID, clock.Now())
		return err
	})
	if err != nil {
		return p, err
	}
	cart, err := s.cart(ctx, p.facts, cartID)
	if err != nil {
		return p, err
	}
	p.basket = basketOf(cart)
	p.lines, err = linesOf(p.basket, refs)
	return p, err
}

// price works the bill out inside tx, which the caller opened in the customer's
// scope. The products and the fee rule are shop data; the address and the
// contact are the caller's own.
func (s *Service) price(ctx context.Context, tx *db.Tx, userID, addressID uuid.UUID, p prepared) (priced, error) {
	var out priced
	var err error
	if out.address, err = s.users.AddressFor(ctx, tx, userID, addressID); err != nil {
		return out, err
	}
	if out.contact, err = s.users.ContactOf(ctx, tx, userID); err != nil {
		return out, err
	}
	rule, err := s.delivery.RuleFor(ctx, tx, p.shopID)
	if err != nil {
		return out, err
	}
	products, err := s.products.BillProducts(ctx, tx, p.shopID, p.basket.ids)
	if err != nil {
		return out, err
	}
	source := "DEFAULT"
	if rule.ShopID != nil {
		source = "SHOP"
	}
	out.Bill, err = billing.Build(billing.BillInput{
		Shop: billing.ShopSnapshot{
			Location: p.facts.Location, MinOrderCents: money.Cents(p.facts.MinOrderCents),
			MinPlatformMarginCents: money.Cents(p.facts.MinPlatformMarginCents), MaxLines: p.facts.MaxLines, Open: p.facts.Open(),
		},
		Lines: p.lines, Products: products, Address: geo.Point{Lat: out.address.Lat, Lng: out.address.Lng},
		FeeRule: rule.Rule, FeeRuleSource: source, PhoneVerified: out.contact.PhoneVerified, Now: clock.Now(),
	})
	return out, err
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
