package billing

import (
	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
	"gp/internal/shared/geo"
	"gp/internal/shared/money"
)

// absoluteMaxQty stops a silly quantity before any product is looked at. A
// product's own max_per_order is never higher (schema: 1 to 999).
const absoluteMaxQty = 999

// Build prices a basket. Prices always come from the products in the input, never
// from the cart or the caller. Lines that are not for sale are listed as excluded
// and left out of every total. Problems with the request itself (an empty basket,
// a bad quantity, too many lines) are errors; facts about the order (shop closed,
// out of range, below the minimum) are in Checks, so a quote can still show the bill.
func Build(in BillInput) (Bill, error) {
	if len(in.Lines) == 0 {
		return Bill{}, apperr.New(apperr.CartEmpty)
	}
	if in.Shop.MaxLines > 0 && len(in.Lines) > in.Shop.MaxLines {
		return Bill{}, &apperr.Error{Code: apperr.QtyLimit, Fields: map[string]string{"lines": "too_many"}}
	}
	bill := Bill{Lines: []BillLine{}, Excluded: []Excluded{}}
	var items, margin money.Cents
	weightG := 0
	seen := map[uuid.UUID]bool{}
	for _, l := range in.Lines {
		if seen[l.ShopProductID] {
			return Bill{}, apperr.Validation(map[string]string{"lines": "duplicate"})
		}
		seen[l.ShopProductID] = true
		pref, err := prefOf(l.SubPref)
		if err != nil {
			return Bill{}, err
		}
		if l.Qty < 1 || l.Qty > absoluteMaxQty {
			return Bill{}, &apperr.Error{Code: apperr.QtyLimit, Fields: map[string]string{"qty": "out_of_range"}}
		}
		p, found := in.Products[l.ShopProductID]
		if !found || !p.Available {
			bill.Excluded = append(bill.Excluded, Excluded{ShopProductID: l.ShopProductID, Name: p.Name, Reason: ReasonItemUnavailable})
			continue
		}
		if l.Qty > p.MaxPerOrder {
			return Bill{}, &apperr.Error{Code: apperr.QtyLimit, Fields: map[string]string{"qty": "over_max_per_order"}}
		}
		line, err := p.price(l, pref)
		if err != nil {
			return Bill{}, err
		}
		perPack, err := money.MulQty(p.OnlinePriceCents-p.ShopPriceCents, int64(l.Qty))
		if err != nil {
			return Bill{}, err
		}
		if items, err = money.Add(items, line.LineCents); err != nil {
			return Bill{}, err
		}
		if margin, err = money.Add(margin, perPack); err != nil {
			return Bill{}, err
		}
		weightG += l.Qty * p.WeightG
		bill.Lines = append(bill.Lines, line)
	}
	return bill.finish(in, items, margin, weightG)
}

// price is the line for a quantity of this product at today's customer price.
func (p ProductSnapshot) price(l LineInput, pref enum.SubstitutionPref) (BillLine, error) {
	total, err := money.MulQty(p.OnlinePriceCents, int64(l.Qty))
	if err != nil {
		return BillLine{}, err
	}
	return BillLine{
		ShopProductID: p.ID, Name: p.Name, Unit: p.Unit, Qty: l.Qty, UnitPriceCents: p.OnlinePriceCents, LineCents: total,
		Status: StatusOK, PriceChanged: l.CartUnitPrice != nil && *l.CartUnitPrice != p.OnlinePriceCents,
		SubPref: pref, ShopPriceCents: p.ShopPriceCents, WeightG: p.WeightG,
	}, nil
}

// prefOf checks the substitution choice; no choice means SIMILAR.
func prefOf(p enum.SubstitutionPref) (enum.SubstitutionPref, error) {
	switch {
	case p == "":
		return enum.SubstitutionPrefSimilar, nil
	case !p.Valid():
		return "", apperr.Validation(map[string]string{"subPref": "invalid"})
	}
	return p, nil
}

// finish adds the fees, the discount and the checks to a priced basket.
func (b Bill) finish(in BillInput, items, margin money.Cents, weightG int) (Bill, error) {
	b.MarginCents, b.WeightG = margin, weightG
	distanceM := geo.HaversineM(in.Shop.Location, in.Address)
	b.DistanceM = distanceM
	inRange := in.FeeRule.InRange(distanceM)
	b.Delivery = Delivery{DistanceKm: delivery.Dec2((distanceM + 5) / 10), WeightKg: delivery.Dec2((weightG + 9) / 10), FeeRule: in.FeeRuleSource}
	b.Checks = Checks{
		ShopOpen: in.Shop.Open, InRange: inRange, MinOrderMet: items >= in.Shop.MinOrderCents, PhoneVerified: in.PhoneVerified,
	}
	t := Totals{ItemsCents: items}
	if len(b.Lines) > 0 {
		if inRange {
			var err error
			if t.DeliveryFeeCents, err = delivery.Fee(in.FeeRule, distanceM, weightG, items); err != nil {
				return Bill{}, err
			}
		}
		t.ServiceFeeCents = max(0, in.Shop.MinPlatformMarginCents-margin)
	}
	if in.Coupon != nil && len(b.Lines) > 0 {
		income, err := money.Add(margin, t.ServiceFeeCents)
		if err != nil {
			return Bill{}, err
		}
		if t.DiscountCents, err = discount(in, items, income); err != nil {
			return Bill{}, err
		}
	}
	total, err := money.Add(items, t.DeliveryFeeCents)
	if err == nil {
		total, err = money.Add(total, t.ServiceFeeCents)
	}
	if err != nil {
		return Bill{}, err
	}
	t.TotalCents = total - t.DiscountCents
	b.Totals = t
	return b, nil
}
