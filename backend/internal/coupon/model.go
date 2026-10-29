// Package coupon holds the coupons the admin makes and the reserve / release /
// confirm steps an order takes through them (docs/03 section 11). The discount
// itself is worked out by billing; this package only decides whether a use may be
// taken, atomically, and keeps the count honest.
package coupon

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/optional"
)

// Coupon is a coupon as the admin sees it.
type Coupon struct {
	ID               uuid.UUID         `json:"id"`
	Code             string            `json:"code"`
	ShopID           *uuid.UUID        `json:"shopId"` // nil: every shop
	Type             enum.CouponType   `json:"type"`
	Value            int64             `json:"value"` // PERCENT: basis points, FIXED: cents
	MaxDiscountCents *money.Cents      `json:"maxDiscountCents"`
	MinOrderCents    money.Cents       `json:"minOrderCents"`
	StartsAt         time.Time         `json:"startsAt"`
	EndsAt           time.Time         `json:"endsAt"`
	UsageLimit       *int              `json:"usageLimit"` // nil: unlimited
	PerUserLimit     int               `json:"perUserLimit"`
	UsedCount        int               `json:"usedCount"`
	FundedBy         enum.FundedBy     `json:"fundedBy"`
	AllowLoss        bool              `json:"allowLoss"`
	Status           enum.CouponStatus `json:"status"`
	CreatedAt        time.Time         `json:"createdAt"`
}

// CreateInput is the body of POST /admin/coupons. What can change later is
// limited (PatchInput): the code, the value and who pays are fixed once made,
// because orders already hold discounts worked out from them.
type CreateInput struct {
	Code             string          `json:"code"`
	ShopID           *uuid.UUID      `json:"shopId"`
	Type             enum.CouponType `json:"type"`
	Value            int64           `json:"value"`
	MaxDiscountCents *money.Cents    `json:"maxDiscountCents"`
	MinOrderCents    money.Cents     `json:"minOrderCents"`
	StartsAt         time.Time       `json:"startsAt"`
	EndsAt           time.Time       `json:"endsAt"`
	UsageLimit       *int            `json:"usageLimit"`
	PerUserLimit     int             `json:"perUserLimit"` // 0 means 1
	FundedBy         enum.FundedBy   `json:"fundedBy"`
	AllowLoss        bool            `json:"allowLoss"`
}

// PatchInput is the body of PATCH /admin/coupons/{id}. A field left out keeps
// its value; null on usageLimit or maxDiscountCents removes the limit.
type PatchInput struct {
	Status           optional.Field[enum.CouponStatus] `json:"status"`
	StartsAt         optional.Field[time.Time]         `json:"startsAt"`
	EndsAt           optional.Field[time.Time]         `json:"endsAt"`
	UsageLimit       optional.Field[int]               `json:"usageLimit"`
	PerUserLimit     optional.Field[int]               `json:"perUserLimit"`
	MaxDiscountCents optional.Field[money.Cents]       `json:"maxDiscountCents"`
	MinOrderCents    optional.Field[money.Cents]       `json:"minOrderCents"`
	AllowLoss        optional.Field[bool]              `json:"allowLoss"`
}

// ListQuery filters GET /admin/coupons.
type ListQuery struct {
	Status string
	ShopID string
}

var codePattern = regexp.MustCompile(`^[A-Z0-9]{4,20}$`)

// Normalize upper-cases and trims a code the way it is stored, and says whether
// it can be a code at all. Codes are case-insensitive (FR-CPN-05).
func Normalize(code string) (string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	return code, codePattern.MatchString(code)
}

const (
	// maxPercentBP is 100.00 %, the same ceiling as the table's check.
	maxPercentBP = 10000
	// maxMoney keeps a fixed value or a cap far below an overflow of int64 sums.
	maxMoney = 1_000_000_000_00
	// maxUses is a ceiling that no real campaign reaches.
	maxUses = 10_000_000
	// maxPerUser is the most uses of one coupon by one customer.
	maxPerUser = 100
	// listMax is how many coupons the list returns.
	listMax = 100
)
