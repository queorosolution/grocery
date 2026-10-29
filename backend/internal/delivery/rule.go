// Package delivery owns delivery fees and delivery slots: the pure fee formula
// (docs/07 section 7), the fee rule and slot admin API, and the atomic slot
// Book / Release queries the order module calls.
package delivery

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
)

// Rule is what the fee formula needs. Zero values mean "no charge" for every
// part, so a fresh rule is just the base fee.
type Rule struct {
	BaseCents      money.Cents  `json:"baseCents"`
	PerKmCents     money.Cents  `json:"perKmCents"`
	FreeKm         Dec2         `json:"freeKm"`
	PerKgCents     money.Cents  `json:"perKgCents"`
	FreeKg         Dec2         `json:"freeKg"`
	FreeAboveCents *money.Cents `json:"freeAboveCents"`
	MinCents       money.Cents  `json:"minCents"`
	MaxCents       *money.Cents `json:"maxCents"`
	MaxKm          Dec2         `json:"maxKm"`
	RoadFactor     Dec2         `json:"roadFactor"`
}

// DefaultRule matches the column defaults of delivery_fee_rules. PUT bodies
// start from it, so a field left out keeps its default.
func DefaultRule() Rule { return Rule{MaxKm: 800, RoadFactor: 130} }

// RuleRow is a stored rule. ShopID is nil for the platform default.
type RuleRow struct {
	ShopID    *uuid.UUID `json:"shopId"`
	Rule      Rule       `json:"rule"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

const maxMoney = 1_000_000_000 // Rs 10 million: far above any fee, small enough to never overflow

// Validate checks a rule against the same limits as the table constraints.
func (r Rule) Validate() error {
	v := validate.New().
		Int("baseCents", int64(r.BaseCents), 0, maxMoney).
		Int("perKmCents", int64(r.PerKmCents), 0, maxMoney).
		Int("perKgCents", int64(r.PerKgCents), 0, maxMoney).
		Int("minCents", int64(r.MinCents), 0, maxMoney).
		Int("freeKm", int64(r.FreeKm), 0, 99999).
		Int("freeKg", int64(r.FreeKg), 0, 999999).
		Int("maxKm", int64(r.MaxKm), 1, 5000).
		Int("roadFactor", int64(r.RoadFactor), 100, 200)
	if r.FreeAboveCents != nil {
		v.Int("freeAboveCents", int64(*r.FreeAboveCents), 1, maxMoney)
	}
	if r.MaxCents != nil {
		v.Int("maxCents", int64(*r.MaxCents), 0, maxMoney)
		v.Check("maxCents", *r.MaxCents >= r.MinCents, "below_min")
	}
	return v.Err()
}

// InRange reports whether an address this far away (straight line, metres) is
// inside the shop's delivery distance once the road factor is applied.
func (r Rule) InRange(distanceM int) bool {
	return int64(distanceM)*int64(r.RoadFactor) <= int64(r.MaxKm)*1000
}

// Fee is the delivery fee for one address and basket (docs/07 section 7).
// distanceM is the straight line; the road factor turns it into road km. All
// steps are integer maths: km and kg are compared in fixed units, never as floats.
func Fee(r Rule, distanceM, weightG int, items money.Cents) (money.Cents, error) {
	if distanceM < 0 || weightG < 0 || items < 0 {
		return 0, apperr.Validation(map[string]string{"input": "negative"})
	}
	if !r.InRange(distanceM) {
		return 0, apperr.New(apperr.OutOfRange)
	}
	if r.FreeAboveCents != nil && items >= *r.FreeAboveCents {
		return 0, nil
	}
	roadM100 := int64(distanceM) * int64(r.RoadFactor) // road metres x 100
	extraKm := ceilPositive(roadM100-int64(r.FreeKm)*1000, 100_000)
	extraKg := ceilPositive(int64(weightG)-int64(r.FreeKg)*10, 1000)

	fee := r.BaseCents
	for _, part := range []struct {
		unit money.Cents
		qty  int64
	}{{r.PerKmCents, extraKm}, {r.PerKgCents, extraKg}} {
		cost, err := money.MulQty(part.unit, part.qty)
		if err != nil {
			return 0, err
		}
		if fee, err = money.Add(fee, cost); err != nil {
			return 0, err
		}
	}
	if fee < r.MinCents {
		fee = r.MinCents
	}
	if r.MaxCents != nil && fee > *r.MaxCents {
		fee = *r.MaxCents
	}
	return fee, nil
}

// ceilPositive is ceil(a / b) for a > 0 and 0 otherwise.
func ceilPositive(a, b int64) int64 {
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}
