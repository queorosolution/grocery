// Package billing is the one place that adds up money for an order (docs/07
// section 8). Build is pure: it reads only its input and never touches a
// database, the clock or the network, so the quote, the submit, a revision and
// a coupon all get the same answer from the same facts.
package billing

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/internal/shared/enum"
	"gp/internal/shared/geo"
	"gp/internal/shared/money"
)

// ShopSnapshot is what Build needs to know about the shop.
type ShopSnapshot struct {
	Location               geo.Point
	MinOrderCents          money.Cents
	MinPlatformMarginCents money.Cents
	MaxLines               int
	Open                   bool // accepting orders and inside its hours, decided by the caller
}

// ProductSnapshot is one shop product as the database has it now. WeightG is
// the pack weight, already resolved by the caller (the product, else its
// category default).
type ProductSnapshot struct {
	ID               uuid.UUID
	Name             string
	Unit             string
	ShopPriceCents   money.Cents
	OnlinePriceCents money.Cents
	Available        bool
	MaxPerOrder      int
	WeightG          int
}

// LineInput is one line the customer asks for. CartUnitPrice is the price the
// cart showed (nil when unknown); it only decides the "price changed" flag and
// never the bill.
type LineInput struct {
	ShopProductID uuid.UUID
	Qty           int
	SubPref       enum.SubstitutionPref
	CartUnitPrice *money.Cents
}

// CouponSnapshot is a coupon the caller has already looked up and checked for
// existence, status, shop and total limit. Build applies the rest.
type CouponSnapshot struct {
	Type             enum.CouponType
	Value            int64 // PERCENT: basis points, FIXED: cents
	MaxDiscountCents *money.Cents
	MinOrderCents    money.Cents
	StartsAt, EndsAt time.Time
	PerUserLimit     int
	FundedBy         enum.FundedBy
	AllowLoss        bool
}

// BillInput is everything Build reads.
type BillInput struct {
	Shop     ShopSnapshot
	Lines    []LineInput
	Products map[uuid.UUID]ProductSnapshot
	Address  geo.Point
	FeeRule  delivery.Rule
	// FeeRuleSource says which rule FeeRule is: "SHOP" or "DEFAULT".
	FeeRuleSource string
	// Coupon is nil before AWAITING_PAYMENT. UserCouponUses is how many times the
	// customer has used it before.
	Coupon         *CouponSnapshot
	UserCouponUses int
	// PhoneVerified is passed through to the checks.
	PhoneVerified bool
	Now           time.Time
}

// Line status values.
const StatusOK = "OK"

// Reasons a requested line is left out of the bill.
const ReasonItemUnavailable = "ITEM_UNAVAILABLE"

// BillLine is one priced line.
type BillLine struct {
	ShopProductID  uuid.UUID             `json:"shopProductId"`
	Name           string                `json:"name"`
	Unit           string                `json:"unit"`
	Qty            int                   `json:"qty"`
	UnitPriceCents money.Cents           `json:"unitPriceCents"`
	LineCents      money.Cents           `json:"lineCents"`
	Status         string                `json:"status"`
	PriceChanged   bool                  `json:"priceChanged"`
	SubPref        enum.SubstitutionPref `json:"subPref"`
	// ShopPriceCents is what the shop charges for one pack. It feeds the ledger
	// (platform margin) and is not part of the customer's view.
	ShopPriceCents money.Cents `json:"-"`
	// WeightG is the pack weight, stored on the order line.
	WeightG int `json:"-"`
}

// Excluded is a requested line that is not in the bill.
type Excluded struct {
	ShopProductID uuid.UUID `json:"shopProductId"`
	Name          string    `json:"name"`
	Reason        string    `json:"reason"`
}

// Totals are the five money figures of an order.
type Totals struct {
	ItemsCents       money.Cents `json:"itemsCents"`
	DeliveryFeeCents money.Cents `json:"deliveryFeeCents"`
	ServiceFeeCents  money.Cents `json:"serviceFeeCents"`
	DiscountCents    money.Cents `json:"discountCents"`
	TotalCents       money.Cents `json:"totalCents"`
}

// Delivery describes how the fee was worked out. Distance is the straight line.
type Delivery struct {
	DistanceKm delivery.Dec2 `json:"distanceKm"`
	WeightKg   delivery.Dec2 `json:"weightKg"`
	FeeRule    string        `json:"feeRule"`
}

// Checks are the yes/no facts the caller turns into errors on submit.
type Checks struct {
	ShopOpen      bool `json:"shopOpen"`
	InRange       bool `json:"inRange"`
	MinOrderMet   bool `json:"minOrderMet"`
	PhoneVerified bool `json:"phoneVerified"`
}

// Bill is the result of Build.
type Bill struct {
	Lines    []BillLine `json:"lines"`
	Excluded []Excluded `json:"excluded"`
	Totals   Totals     `json:"totals"`
	Delivery Delivery   `json:"delivery"`
	Checks   Checks     `json:"checks"`
	// MarginCents is the platform margin of the items (online price minus shop
	// price). It is kept for the ledger and for the coupon cap, not shown.
	MarginCents money.Cents `json:"-"`
	// DistanceM and WeightG are the straight-line distance and the pack weight the
	// fee was worked out from, stored on the order.
	DistanceM int `json:"-"`
	WeightG   int `json:"-"`
}
