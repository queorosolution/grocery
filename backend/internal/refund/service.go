// Package refund owns refund requests (docs/03 section 11, FR-RFD-01..05): the
// shop asks, the admin decides and later marks the money as sent, and only then
// does the ledger take the refund back.
package refund

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/finance"
	"gp/internal/order"
	"gp/internal/shared/db"
)

// Orders is what refunds need from the order module. *order.Service is the real one.
type Orders interface {
	RefundOrder(ctx context.Context, tx *db.Tx, scope db.Scope, orderID uuid.UUID) (order.RefundOrder, error)
	MarkLineMissing(ctx context.Context, tx *db.Tx, scope db.Scope, orderID, itemID uuid.UUID) (order.RefundLine, error)
	RestoreLine(ctx context.Context, tx *db.Tx, orderID, itemID uuid.UUID) error
	LineOf(ctx context.Context, tx *db.Tx, orderID, itemID uuid.UUID) (order.RefundLine, error)
	CancelRefund(ctx context.Context, tx *db.Tx, scope db.Scope, orderID, refundID uuid.UUID, reason string) error
}

// Ledger posts the rows of a refund that was paid out. *finance.Service is the real one.
type Ledger interface {
	PostRefund(ctx context.Context, tx *db.Tx, f finance.RefundFacts) error
}

type Service struct {
	db     *db.DB
	ledger Ledger
	orders Orders
}

func NewService(d *db.DB, l Ledger) *Service { return &Service{db: d, ledger: l} }

// SetOrders gives the service the order module. It is a setter because the order
// module needs the refund service too (T19).
func (s *Service) SetOrders(o Orders) { s.orders = o }
