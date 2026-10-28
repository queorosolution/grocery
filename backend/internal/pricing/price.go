// Package pricing turns a shop's own price into the customer price and keeps
// the two in step (docs/07 section 6). OnlinePrice is pure; the service writes.
package pricing

import (
	"errors"

	"gp/internal/shared/money"
)

// ErrMRPBelowShopPrice means the rules leave no valid customer price: it would
// be under what the shop charges. It is a data problem, not a bug.
var ErrMRPBelowShopPrice = errors.New("pricing: online price would be below the shop price")

// ErrBadInput is a price or markup outside what the database allows.
var ErrBadInput = errors.New("pricing: price or markup out of range")

// MaxMarkupBP is the largest markup, 20.00%, as in the schema.
const MaxMarkupBP = 2000

// OnlinePrice returns the customer price in cents.
//
//  1. raw = shopPrice x (10000 + markupBP) / 10000, rounded half up to the cent
//  2. price = raw rounded half up to a whole rupee
//  3. never more than the MRP, when there is one
//  4. never less than the shop price, else ErrMRPBelowShopPrice
func OnlinePrice(shopPrice money.Cents, mrp *money.Cents, markupBP int) (money.Cents, error) {
	if shopPrice <= 0 || markupBP < 0 || markupBP > MaxMarkupBP || (mrp != nil && *mrp <= 0) {
		return 0, ErrBadInput
	}
	raw, err := money.ApplyBP(shopPrice, 10000+markupBP)
	if err != nil {
		return 0, err
	}
	price, err := money.RoundRupee(raw)
	if err != nil {
		return 0, err
	}
	if mrp != nil && price > *mrp {
		price = *mrp
	}
	if price < shopPrice {
		return 0, ErrMRPBelowShopPrice
	}
	return price, nil
}
