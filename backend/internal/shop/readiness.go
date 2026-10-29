package shop

import (
	"time"

	"gp/internal/shared/enum"
)

// Missing names one thing a shop still needs before it can take orders.
type Missing string

const (
	MissingStatus  Missing = "SHOP_ACTIVE"     // status is not ACTIVE
	MissingHours   Missing = "HOURS"           // no open day at all
	MissingPayment Missing = "PAYMENT_ACCOUNT" // no usable payment method
	MissingSlot    Missing = "FUTURE_SLOT"     // slot shops need one bookable slot
)

// Readiness answers "can this shop be switched on?". Open-now and the accepting
// switch are not part of it: those are the live state, not the setup.
type Readiness struct {
	Ready   bool      `json:"ready"`
	Missing []Missing `json:"missing"`
}

// snapshot is everything the rules need, loaded once.
type snapshot struct {
	Status        enum.ShopStatus
	Accepting     bool
	DeliveryType  enum.DeliveryType
	Hours         weekHours
	PaymentUsable bool
	HasFutureSlot bool
}

// setup lists what is missing, in a stable order.
func (s snapshot) setup() Readiness {
	missing := []Missing{}
	if s.Status != enum.ShopStatusActive {
		missing = append(missing, MissingStatus)
	}
	if len(s.Hours) == 0 {
		missing = append(missing, MissingHours)
	}
	if !s.PaymentUsable {
		missing = append(missing, MissingPayment)
	}
	if s.DeliveryType == enum.DeliveryTypeSlot && !s.HasFutureSlot {
		missing = append(missing, MissingSlot)
	}
	return Readiness{Ready: len(missing) == 0, Missing: missing}
}

// Verdict is why a shop cannot take an order right now. The zero value is "it can".
type Verdict string

const (
	CanOrder Verdict = ""
	Closed   Verdict = "SHOP_CLOSED"    // switched off, or outside the opening hours
	NotReady Verdict = "SHOP_NOT_READY" // setup is incomplete
)

// canTakeOrders is FR-SHOP-04: active, accepting, open now, a usable payment
// method and, for slot delivery, at least one future slot.
func (s snapshot) canTakeOrders(now time.Time) Verdict {
	if !s.setup().Ready {
		return NotReady
	}
	if !s.Accepting || !s.Hours.openAt(now) {
		return Closed
	}
	return CanOrder
}
