package refund

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// Refund is a refund as the admin sees it.
type Refund struct {
	ID          uuid.UUID         `json:"id"`
	OrderID     uuid.UUID         `json:"orderId"`
	OrderCode   string            `json:"orderCode"`
	ShopID      uuid.UUID         `json:"shopId"`
	Type        enum.RefundType   `json:"type"`
	OrderItemID *uuid.UUID        `json:"orderItemId"`
	AmountCents money.Cents       `json:"amountCents"`
	Reason      string            `json:"reason"`
	Status      enum.RefundStatus `json:"status"`
	Method      enum.RefundMethod `json:"method"`
	Reference   *string           `json:"reference"`
	// RequestedBy is the shop member who asked; it is null for a refund the system or the admin made.
	RequestedBy *uuid.UUID `json:"requestedBy"`
	DecidedAt   *time.Time `json:"decidedAt"`
	PaidOutAt   *time.Time `json:"paidOutAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// RequestInput is the body of the shop's refund request.
type RequestInput struct {
	Type        enum.RefundType `json:"type"`
	OrderItemID *uuid.UUID      `json:"orderItemId"`
	Reason      string          `json:"reason"`
}

// DecideInput is the body of the admin's decision. Approve has no default: a body
// without it is refused, so a missing field never rejects a refund.
type DecideInput struct {
	Approve *bool  `json:"approve"`
	Note    string `json:"note"`
}

// PaidOutInput is the body that says the money was sent back to the customer.
type PaidOutInput struct {
	Reference string `json:"reference"`
}
