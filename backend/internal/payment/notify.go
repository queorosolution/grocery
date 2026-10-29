package payment

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"

	"gp/internal/refund"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/logx"
	"gp/internal/shop"
)

// NotifyPattern is the PayHere callback route (docs/06 section 7). It is signed.
const NotifyPattern = "/webhooks/payhere"

// PayHere status codes (docs/09 section 2.3).
const (
	codeSuccess    = 2
	codePending    = 0
	codeCancelled  = -1
	codeFailed     = -2
	codeChargeback = -3
)

// notifyFields must all be present (check 1).
var notifyFields = []string{"merchant_id", "order_id", "payment_id", "payhere_amount", "payhere_currency", "status_code", "md5sig"}

// checked is a callback that passed checks 1 to 7. Without a payment it is one
// we do not know: it is answered and ignored.
type checked struct {
	payment Payment
	result  Notified
	// mismatch is set for a signed callback that does not fit the payment.
	mismatch bool
}

// Verify is the signature check of POST /webhooks/payhere, run by the route guard
// before the handler. It fails (400) for a callback with a missing field or a
// wrong signature, and passes anything else, including a payment we do not know,
// which the handler then ignores.
func (s *Service) Verify(ctx context.Context, form url.Values) error {
	_, err := s.check(ctx, form)
	return err
}

// check runs checks 1 to 7 of docs/09 section 2.3, in that order.
func (s *Service) check(ctx context.Context, form url.Values) (checked, error) {
	var c checked
	fields := map[string]string{}
	for _, f := range notifyFields {
		if form.Get(f) == "" {
			fields[f] = "required"
		}
	}
	if len(fields) > 0 {
		return c, apperr.Validation(fields)
	}
	id, err := uuid.Parse(form.Get("order_id"))
	if err != nil {
		return c, nil // not one of our payment ids
	}
	var acc shop.PayAccount
	var known bool
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if c.payment, known, err = paymentByID(ctx, tx, id); err != nil || !known {
			return err
		}
		if c.payment.AccountID == nil {
			known = false
			return nil
		}
		acc, known, err = s.accounts.PayAccountByID(ctx, tx, *c.payment.AccountID)
		return err
	})
	if err != nil {
		return c, err
	}
	notifier, ok := s.providers[c.payment.Provider].(Notifier)
	if !known || !ok {
		return checked{}, nil
	}
	c.result, err = notifier.ParseNotify(ctx, acc, c.payment, form)
	switch {
	case errors.Is(err, ErrBadSignature):
		// No secret, no signature and no body in the log: only that it happened.
		logx.From(ctx).Warn("payhere.bad_signature", "payment", c.payment.ID)
		return checked{}, apperr.New(apperr.ValidationError)
	case errors.Is(err, ErrMismatch):
		logx.From(ctx).Error("payhere.mismatch", "payment", c.payment.ID, "order", c.payment.OrderID)
		return checked{payment: c.payment, mismatch: true}, nil
	case err != nil:
		return checked{}, err
	}
	return c, nil
}

// HandleNotify acts on a PayHere callback (docs/09 section 2.3, checks 1 to 11).
// It answers nil for everything that is not an error of ours: a payment we do
// not know, a replay and a callback that does not match are logged, not retried.
func (s *Service) HandleNotify(ctx context.Context, form url.Values) error {
	c, err := s.check(ctx, form)
	if err != nil || c.payment.ID == uuid.Nil {
		return err
	}
	if c.mismatch {
		return nil // signed, but not for this payment: it pays nothing
	}
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		p, found, err := lockPayment(ctx, tx, c.payment.ID)
		if err != nil || !found {
			return err
		}
		return s.apply(ctx, tx, p, c.result)
	})
	if db.IsUniqueViolation(err) {
		// The provider's own reference is already on another payment: a replay.
		logx.From(ctx).Warn("payhere.duplicate_ref", "payment", c.payment.ID)
		return nil
	}
	return err
}

// apply acts on the status code of a verified callback (check 9).
func (s *Service) apply(ctx context.Context, tx *db.Tx, p Payment, n Notified) error {
	switch n.Code {
	case codeSuccess:
		return s.succeeded(ctx, tx, p, n)
	case codePending:
		return nil // keep INITIATED; PayHere calls again when it knows
	case codeCancelled:
		return s.leave(ctx, tx, p, enum.PaymentStatusCancelled, n)
	case codeFailed:
		return s.leave(ctx, tx, p, enum.PaymentStatusFailed, n)
	case codeChargeback:
		// The money is going back to the customer: the admin must see it.
		logx.From(ctx).Error("payhere.chargeback", "payment", p.ID, "order", p.OrderID, "shop", p.ShopID)
		return settle(ctx, tx, p.ID, enum.PaymentStatusChargeback, n.Ref, n.Method, n.Raw)
	}
	logx.From(ctx).Warn("payhere.unknown_status", "payment", p.ID, "code", n.Code)
	return nil
}

// leave ends an attempt that did not pay. A payment that already succeeded stays so.
func (s *Service) leave(ctx context.Context, tx *db.Tx, p Payment, to enum.PaymentStatus, n Notified) error {
	if p.Status != enum.PaymentStatusInitiated {
		return nil
	}
	return settle(ctx, tx, p.ID, to, n.Ref, n.Method, n.Raw)
}

// succeeded is code 2. The payment is SUCCESS; the order is paid if it still waits
// for this money. Money that arrives for an order that cannot take it is never
// dropped: it becomes a refund request (BIZ-09).
func (s *Service) succeeded(ctx context.Context, tx *db.Tx, p Payment, n Notified) error {
	if p.Status == enum.PaymentStatusSuccess {
		if p.ProviderRef != nil && *p.ProviderRef == n.Ref {
			return nil // a replay (BIZ-08): one payment, one ledger set
		}
		return s.refundExtra(ctx, tx, p, enum.RefundTypeFull, "Paid twice for one payment")
	}
	if p.Status == enum.PaymentStatusChargeback {
		return nil
	}
	if err := settle(ctx, tx, p.ID, enum.PaymentStatusSuccess, n.Ref, n.Method, n.Raw); err != nil {
		return err
	}
	sys := db.System()
	st, err := s.orders.PayState(ctx, tx, sys, p.OrderID)
	if err != nil {
		return err
	}
	switch {
	case st.Status != enum.OrderStatusAwaitingPayment:
		return s.refundExtra(ctx, tx, p, enum.RefundTypeLatePayment, fmt.Sprintf("Payment arrived when the order was %s", st.Status))
	case st.TotalCents != p.AmountCents:
		// A coupon changed the total after this attempt started.
		return s.refundExtra(ctx, tx, p, enum.RefundTypeFull, "The order total changed before the payment arrived")
	}
	return s.orders.MarkPaid(ctx, tx, sys, p.OrderID, st.Version)
}

func (s *Service) refundExtra(ctx context.Context, tx *db.Tx, p Payment, kind enum.RefundType, reason string) error {
	_, err := s.refunds.Open(ctx, tx, refund.OpenInput{OrderID: p.OrderID, ShopID: p.ShopID, PaymentID: p.ID, Type: kind, AmountCents: p.AmountCents, Reason: reason})
	return err
}
