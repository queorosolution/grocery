package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/order"
	"gp/internal/refund"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/idem"
	"gp/internal/shared/ratelimit"
	"gp/internal/shop"
)

// What the payment module needs from the others: their services, never their repos.

type Orders interface {
	Payable(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID, version int) (order.Payable, error)
	PayState(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID) (order.PayState, error)
	MarkPaid(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID, version int) error
	Get(ctx context.Context, scope db.Scope, id uuid.UUID) (order.Detail, error)
}

type Accounts interface {
	PayAccountOf(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (shop.PayAccount, error)
	PayAccountByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (shop.PayAccount, bool, error)
}

type Refunds interface {
	Open(ctx context.Context, tx *db.Tx, in refund.OpenInput) (uuid.UUID, error)
}

type Service struct {
	db        *db.DB
	orders    Orders
	accounts  Accounts
	refunds   Refunds
	providers Registry
}

func NewService(d *db.DB, orders Orders, accounts Accounts, refunds Refunds, providers Registry) *Service {
	return &Service{db: d, orders: orders, accounts: accounts, refunds: refunds, providers: providers}
}

const (
	// startRoute scopes the idempotency key: one key is one payment attempt.
	startRoute = "POST /orders/{id}/payment"
	// attemptsPerHour keeps a customer from filling the table with attempts.
	attemptsPerHour = 20
	maxReference    = 100
)

// StartInput is the body of POST /orders/{id}/payment: the order version the customer saw.
type StartInput struct {
	Version int `json:"version"`
}

// Start begins a payment attempt for an order that waits for payment
// (FR-PAY-01). It saves an INITIATED payment and returns what the browser needs:
// the signed PayHere form, or the bank details. A retry with the same key gets
// the same answer and no second attempt.
func (s *Service) Start(ctx context.Context, scope db.Scope, orderID uuid.UUID, key string, in StartInput) (json.RawMessage, error) {
	if in.Version < 1 {
		return nil, apperr.Validation(map[string]string{"version": "invalid"})
	}
	hash, err := idem.Hash(struct {
		OrderID uuid.UUID
		Version int
	}{orderID, in.Version})
	if err != nil {
		return nil, err
	}
	if stored, err := s.replay(ctx, scope, key, hash); stored != nil || err != nil {
		return stored, err
	}
	if err := ratelimit.Hit(ctx, s.db, "pay:"+scope.UserID.String(), attemptsPerHour, time.Hour); err != nil {
		return nil, err
	}
	acc, err := s.accountFor(ctx, scope, orderID, in.Version)
	if err != nil {
		return nil, err
	}
	var out json.RawMessage
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		out, err = s.begin(ctx, tx, scope, orderID, key, hash, in, acc)
		return err
	})
	return out, err
}

func (s *Service) replay(ctx context.Context, scope db.Scope, key string, hash []byte) (json.RawMessage, error) {
	var stored *idem.Stored
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		stored, err = idem.Lookup(ctx, tx, scope.UserID, startRoute, key, hash)
		return err
	})
	if stored == nil {
		return nil, err
	}
	return stored.Body, err
}

// accountFor finds the account the order is paid into. Payment accounts are for
// the system scope only, so the shop comes from a read in the customer's own
// scope (which also proves the order is theirs and can be paid), and the
// account from a second read in the system's. Nothing of the customer is read there.
func (s *Service) accountFor(ctx context.Context, scope db.Scope, orderID uuid.UUID, version int) (shop.PayAccount, error) {
	var o order.Payable
	if err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		o, err = s.orders.Payable(ctx, tx, scope, orderID, version)
		return err
	}); err != nil {
		return shop.PayAccount{}, err
	}
	var acc shop.PayAccount
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		acc, err = s.accounts.PayAccountOf(ctx, tx, o.ShopID)
		return err
	})
	// The order carries the provider the customer was shown at submit. A shop
	// that switched since cannot take it by the other way.
	if err == nil && acc.Provider != o.Provider {
		err = apperr.New(apperr.PaymentNotAvailable)
	}
	return acc, err
}

func (s *Service) begin(ctx context.Context, tx *db.Tx, scope db.Scope, orderID uuid.UUID, key string, hash []byte, in StartInput, acc shop.PayAccount) (json.RawMessage, error) {
	if stored, err := idem.Claim(ctx, tx, scope.UserID, startRoute, key, hash); stored != nil || err != nil {
		if stored != nil {
			return stored.Body, nil
		}
		return nil, err
	}
	o, err := s.orders.Payable(ctx, tx, scope, orderID, in.Version)
	if err != nil {
		return nil, err
	}
	prov, ok := s.providers[acc.Provider]
	if !ok || acc.Provider != o.Provider {
		return nil, apperr.New(apperr.PaymentNotAvailable)
	}
	p, err := insertAttempt(ctx, tx, o.ID, o.ShopID, acc.Provider, acc.ID, o.TotalCents)
	if err != nil {
		return nil, err
	}
	checkout, err := prov.Checkout(ctx, acc, p, o)
	if errors.Is(err, ErrNoBank) {
		return nil, apperr.New(apperr.PaymentNotAvailable)
	}
	if err != nil {
		return nil, apperr.New(apperr.Internal).Wrap(err)
	}
	if err := idem.Save(ctx, tx, scope.UserID, startRoute, key, http.StatusOK, checkout); err != nil {
		return nil, err
	}
	return json.Marshal(checkout)
}

// MarkPaidInput is the body of POST /admin/orders/{id}/mark-paid. Reference is
// the bank's reference of the transfer.
type MarkPaidInput struct {
	Version   int    `json:"version"`
	Reference string `json:"reference"`
}

func (in MarkPaidInput) validate() error {
	n := utf8.RuneCountInString(in.Reference)
	switch {
	case in.Version < 1:
		return apperr.Validation(map[string]string{"version": "invalid"})
	case n < 3 || n > maxReference || in.Reference != strings.TrimSpace(in.Reference):
		return apperr.Validation(map[string]string{"reference": "invalid"})
	}
	return nil
}

// MarkPaid is how a bank transfer becomes a paid order (T10, manual mode). The
// admin gives the version they saw and the bank's reference. The payment, the
// move to PAID, the ledger rows and the audit row are one transaction. Only an
// order of a MANUAL shop can be paid this way: a PayHere order is paid by PayHere.
func (s *Service) MarkPaid(ctx context.Context, scope db.Scope, orderID uuid.UUID, in MarkPaidInput) (order.Detail, error) {
	if err := in.validate(); err != nil {
		return order.Detail{}, err
	}
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		st, err := s.orders.PayState(ctx, tx, scope, orderID)
		if err != nil {
			return err
		}
		if st.Provider != enum.PaymentProviderManual {
			return apperr.New(apperr.InvalidTransition)
		}
		// The move first: it checks the version, the status and who may make it.
		if err := s.orders.MarkPaid(ctx, tx, scope, orderID, in.Version); err != nil {
			return err
		}
		paymentID, err := settleManual(ctx, tx, orderID, st.ShopID, st.TotalCents, in.Reference)
		if db.IsUniqueViolation(err) {
			return apperr.Validation(map[string]string{"reference": "used"})
		}
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "order.mark_paid", Entity: "order", EntityID: orderID.String(), ShopID: st.ShopID,
			After: map[string]any{"paymentId": paymentID, "reference": in.Reference}})
	})
	if err != nil {
		return order.Detail{}, err
	}
	return s.orders.Get(ctx, scope, orderID)
}
