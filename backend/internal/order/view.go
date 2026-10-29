package order

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// Viewer says whose eyes an order is shown to. Every field that only one kind of
// viewer may see is added by that viewer's mapper and nowhere else (AUTHZ-06).
type Viewer int

const (
	ViewCustomer Viewer = iota + 1
	ViewConsole         // shop owner, staff and the super admin
	ViewDriver
)

// viewerOf maps the caller's role to the view it may have; nil means none.
func viewerOf(sc db.Scope) (Viewer, bool) {
	switch sc.Role {
	case enum.RoleCustomer:
		return ViewCustomer, true
	case enum.RoleShopOwner, enum.RoleShopStaff, enum.RoleSuperAdmin:
		return ViewConsole, true
	case enum.RoleShopDriver, enum.RolePlatformDriver:
		return ViewDriver, true
	}
	return 0, false
}

type ShopRef struct {
	ID    uuid.UUID `json:"id"`
	Slug  string    `json:"slug"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

type SlotRef struct {
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
}

// AddressView is the delivery address. The pin is for the driver only.
type AddressView struct {
	Label    string   `json:"label"`
	Line1    string   `json:"line1"`
	Landmark *string  `json:"landmark"`
	Lat      *float64 `json:"lat,omitempty"`
	Lng      *float64 `json:"lng,omitempty"`
}

// LineView is one order line. The driver's list has no prices.
type LineView struct {
	ID             uuid.UUID             `json:"id"`
	Name           string                `json:"name"`
	Unit           string                `json:"unit"`
	Qty            int                   `json:"qty"`
	UnitPriceCents *money.Cents          `json:"unitPriceCents,omitempty"`
	LineCents      *money.Cents          `json:"lineCents,omitempty"`
	State          string                `json:"state"`
	SubPref        enum.SubstitutionPref `json:"subPref"`
	RevisionNo     int                   `json:"revisionNo"`
	ReplacesItemID *uuid.UUID            `json:"replacesItemId"`
}

type CouponRef struct {
	Code          string      `json:"code"`
	DiscountCents money.Cents `json:"discountCents"`
}

type PaymentRef struct {
	Provider enum.PaymentProvider `json:"provider"`
	Status   *string              `json:"status"`
}

type Step struct {
	Status enum.OrderStatus `json:"status"`
	At     time.Time        `json:"at"`
}

// CustomerRef is who to call. Console sees the phone masked; the full number is
// shown only while the order is being picked, and to the driver.
type CustomerRef struct {
	Name        string `json:"name"`
	Phone       string `json:"phone,omitempty"`
	PhoneMasked string `json:"phoneMasked,omitempty"`
}

// Detail is the order as one viewer sees it (docs/06 section 6). One struct
// with optional parts keeps the three views the same shape where they overlap.
type Detail struct {
	ID            uuid.UUID        `json:"id"`
	Code          string           `json:"code"`
	Status        enum.OrderStatus `json:"status"`
	Version       int              `json:"version"`
	RevisionNo    int              `json:"revisionNo"`
	Shop          ShopRef          `json:"shop"`
	Slot          *SlotRef         `json:"slot"`
	Address       AddressView      `json:"address"`
	Customer      *CustomerRef     `json:"customer,omitempty"`
	Lines         []LineView       `json:"lines"`
	PreviousLines []LineView       `json:"previousLines,omitempty"`
	Totals        *billing.Totals  `json:"totals,omitempty"`
	TotalCents    *money.Cents     `json:"totalCents,omitempty"`
	Coupon        *CouponRef       `json:"coupon,omitempty"`
	Payment       *PaymentRef      `json:"payment,omitempty"`
	DeliveryCode  *string          `json:"deliveryCode,omitempty"`
	ExpiresAt     *time.Time       `json:"expiresAt,omitempty"`
	Timeline      []Step           `json:"timeline,omitempty"`
	Actions       []Action         `json:"allowedActions"`
}

// orderData is everything the mappers choose from.
type orderData struct {
	orderRow
	Address    addressSnapshot
	Lat, Lng   float64
	Shop       ShopRef
	Slot       *SlotRef
	CustomerNm string
	Totals     billing.Totals
	Coupon     *CouponRef
	Payment    PaymentRef
	PaidAt     *time.Time
	// DeliveryCode is the delivery code, read only for the customer while the order is out.
	DeliveryCode *string
	Items        []LineView
	Steps        []Step
	ExpiresAt    *time.Time
}

// Detail maps the order for a viewer. The role and the user decide the action list.
func (d orderData) Detail(v Viewer, sc db.Scope) Detail {
	out := Detail{ID: d.ID, Code: d.Code, Status: d.Status, Version: d.Version, RevisionNo: d.RevisionNo, Shop: d.Shop, Slot: d.Slot,
		Address: AddressView{Label: d.Address.Label, Line1: d.Address.Line1, Landmark: d.Address.Landmark},
		Lines:   []LineView{}, Actions: d.actions(v, sc)}
	current, previous := d.split()
	switch v {
	case ViewCustomer:
		out.Lines, out.PreviousLines = current, previous
		out.Totals, out.Coupon, out.Payment = &d.Totals, d.Coupon, &d.Payment
		out.ExpiresAt, out.Timeline = d.ExpiresAt, d.Steps
		out.DeliveryCode = d.DeliveryCode // DATA-04: no other viewer is ever given it
	case ViewConsole:
		out.Lines, out.PreviousLines = current, previous
		out.Totals, out.Coupon, out.Payment = &d.Totals, d.Coupon, &d.Payment
		out.ExpiresAt, out.Timeline = d.ExpiresAt, d.Steps
		out.Customer = &CustomerRef{Name: d.CustomerNm, PhoneMasked: maskPhone(d.Address.Phone)}
		if d.Status == enum.OrderStatusPicking {
			out.Customer.Phone = d.Address.Phone
		}
	case ViewDriver:
		out.RevisionNo = 0 // drivers do not see revisions
		out.Lines = withoutPrices(current)
		out.TotalCents = &d.Totals.TotalCents
		out.Customer = &CustomerRef{Name: d.CustomerNm, Phone: d.Address.Phone}
		out.Address.Lat, out.Address.Lng = &d.Lat, &d.Lng
	}
	return out
}

// split separates the lines of the current revision from the earlier ones.
func (d orderData) split() (current, previous []LineView) {
	current, previous = []LineView{}, []LineView{}
	for _, l := range d.Items {
		if l.RevisionNo < d.RevisionNo {
			previous = append(previous, l)
		} else {
			current = append(current, l)
		}
	}
	return current, previous
}

func withoutPrices(lines []LineView) []LineView {
	out := make([]LineView, len(lines))
	for i, l := range lines {
		l.UnitPriceCents, l.LineCents = nil, nil
		out[i] = l
	}
	return out
}

// maskPhone keeps the country code and the last three digits: +94******567.
func maskPhone(p string) string {
	if len(p) < 7 {
		return strings.Repeat("*", len(p))
	}
	return p[:3] + strings.Repeat("*", len(p)-6) + p[len(p)-3:]
}
