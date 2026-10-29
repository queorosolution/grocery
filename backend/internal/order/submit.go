package order

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/idem"
	"gp/internal/shared/logx"
	"gp/internal/shared/ratelimit"
	"gp/internal/shop"
)

const (
	// submitRoute names the idempotency key's scope: one key is one bill.
	submitRoute = "POST /bills"
	// maxOpenUnpaid is FR-BILL-03: a customer may wait on this many bills at once.
	maxOpenUnpaid = 3
	// billsPerHour is BIZ-12.
	billsPerHour = 10
	maxNote      = 300
)

// SubmitInput is the body of POST /bills: the quote body, the slot, and a note.
type SubmitInput struct {
	QuoteInput
	Note *string `json:"note"`
}

func (in SubmitInput) validate() error {
	if err := in.QuoteInput.validate(); err != nil {
		return err
	}
	if in.Note != nil && utf8.RuneCountInString(*in.Note) > maxNote {
		return apperr.Validation(map[string]string{"note": "too_long"})
	}
	return nil
}

// Submit is T01: the customer sends the bill to the shop (FR-BILL-02, FR-BILL-03).
// It answers with the order as JSON. A retry with the same key and body gets the
// same bytes back, even after the cart is gone. The key, the slot place and the
// order are one transaction, so they all happen or none does.
func (s *Service) Submit(ctx context.Context, scope db.Scope, key string, in SubmitInput) (json.RawMessage, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	hash, err := idem.Hash(in)
	if err != nil {
		return nil, err
	}
	if stored, err := s.replay(ctx, scope, key, hash); stored != nil || err != nil {
		return stored, err
	}
	if err := ratelimit.Hit(ctx, s.db, "bills:"+scope.UserID.String(), billsPerHour, time.Hour); err != nil {
		return nil, err
	}
	p, err := s.prepare(ctx, in.ShopID, in.CartID, in.Lines)
	if err != nil {
		// A retry that raced the first request finds the cart already deleted;
		// the first request has finished by then, so its answer is there now.
		if stored, _ := s.replay(ctx, scope, key, hash); stored != nil {
			return stored, nil
		}
		return nil, err
	}
	if err := shopChecks(p.facts, in.SlotID); err != nil {
		return nil, err
	}
	var out json.RawMessage
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		var err error
		out, err = s.place(ctx, tx, scope, key, hash, in, p)
		return err
	})
	return out, err
}

func (s *Service) replay(ctx context.Context, scope db.Scope, key string, hash []byte) (json.RawMessage, error) {
	var stored *idem.Stored
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		stored, err = idem.Lookup(ctx, tx, scope.UserID, submitRoute, key, hash)
		return err
	})
	if stored == nil {
		return nil, err
	}
	return stored.Body, err
}

// shopChecks are the checks that need only the shop: it can take orders now, and a
// slot shop is given a slot while a courier shop is not.
func shopChecks(f shop.OrderFacts, slot *uuid.UUID) error {
	switch f.Verdict {
	case shop.Closed:
		return apperr.New(apperr.ShopClosed)
	case shop.NotReady:
		return apperr.New(apperr.ShopNotReady)
	}
	switch {
	case f.DeliveryType == enum.DeliveryTypeSlot && slot == nil:
		return apperr.Validation(map[string]string{"slotId": "required"})
	case f.DeliveryType != enum.DeliveryTypeSlot && slot != nil:
		return apperr.Validation(map[string]string{"slotId": "not_used"})
	}
	return nil
}

// place does the work of T01 in the caller's transaction.
func (s *Service) place(ctx context.Context, tx *db.Tx, scope db.Scope, key string, hash []byte, in SubmitInput, p prepared) (json.RawMessage, error) {
	if stored, err := idem.Claim(ctx, tx, scope.UserID, submitRoute, key, hash); stored != nil || err != nil {
		if stored != nil {
			return stored.Body, nil
		}
		return nil, err
	}
	if err := lockCustomer(ctx, tx, scope.UserID); err != nil {
		return nil, err
	}
	bill, err := s.price(ctx, tx, scope.UserID, in.AddressID, p)
	if err != nil {
		return nil, err
	}
	if err := s.billChecks(ctx, tx, scope.UserID, bill); err != nil {
		return nil, err
	}
	now := clock.Now()
	if in.SlotID != nil {
		if err := s.delivery.BookSlot(ctx, tx, p.shopID, *in.SlotID, now); err != nil {
			return nil, err
		}
	}
	id, err := s.insert(ctx, tx, scope, in, p, bill)
	if err != nil {
		return nil, err
	}
	d, err := s.detail(ctx, tx, scope, id)
	if err != nil {
		return nil, err
	}
	if err := idem.Save(ctx, tx, scope.UserID, submitRoute, key, http.StatusCreated, d); err != nil {
		return nil, err
	}
	tenant, cartID := p.facts.CartTenant, in.CartID
	tx.AfterCommit(func(ctx context.Context) error {
		// The order is saved; a cart that stays is only clutter and the cart service expires it.
		if err := s.carts.DeleteCart(ctx, tenant, cartID); err != nil {
			logx.From(ctx).Warn("order.cart_delete_failed", "order", id, "err", err)
		}
		return nil
	})
	return json.Marshal(d)
}

// billChecks turns the facts of a priced bill into the refusals of FR-BILL-03.
func (s *Service) billChecks(ctx context.Context, tx *db.Tx, userID uuid.UUID, b priced) error {
	if !b.Checks.PhoneVerified {
		return apperr.New(apperr.PhoneNotVerified)
	}
	open, err := openUnpaid(ctx, tx, userID)
	if err != nil {
		return err
	}
	switch {
	case open >= maxOpenUnpaid:
		return apperr.New(apperr.TooManyOpenOrders)
	case len(b.Excluded) > 0 || len(b.Lines) == 0:
		return apperr.New(apperr.ItemUnavailable)
	case !b.Checks.InRange:
		return apperr.New(apperr.OutOfRange)
	case !b.Checks.MinOrderMet:
		return apperr.New(apperr.BelowMinOrder)
	}
	return nil
}

// insert writes the order, its lines and the first history row, then runs the
// T01 hook. It returns the new order id.
func (s *Service) insert(ctx context.Context, tx *db.Tx, scope db.Scope, in SubmitInput, p prepared, b priced) (uuid.UUID, error) {
	n := newOrder{
		ShopID: p.shopID, CustomerID: scope.UserID, ShopSlug: p.facts.Slug, DeliveryType: p.facts.DeliveryType,
		PaymentProvider: p.facts.PaymentProvider, Lat: b.address.Lat, Lng: b.address.Lng, SlotID: in.SlotID, CartRef: in.CartID,
		Address: addressSnapshot{Label: b.address.Label, Line1: b.address.Line1, Landmark: b.address.Landmark, Phone: b.contact.Phone},
		Bill:    b.Bill,
	}
	if in.Note != nil {
		if note := strings.TrimSpace(*in.Note); note != "" {
			n.Note = &note
		}
	}
	id, code, err := insertOrder(ctx, tx, n)
	if err != nil {
		return uuid.Nil, err
	}
	if err := insertItems(ctx, tx, id, p.shopID, b.Lines); err != nil {
		return uuid.Nil, err
	}
	o := orderRow{ID: id, Code: code, ShopID: p.shopID, CustomerID: scope.UserID, Status: enum.OrderStatusSubmitted, Version: 1, SlotID: in.SlotID}
	if err := hooks["T01"](ctx, s, tx, o, TransitionInput{}); err != nil {
		return uuid.Nil, err
	}
	return id, s.record(ctx, tx, o, nil, enum.OrderStatusSubmitted, actorOf(scope), "")
}
