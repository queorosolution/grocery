package order

import (
	"context"
	"errors"
	"math"
	"regexp"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/crypt"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/secrets"
	"gp/internal/shared/validate"
)

const (
	codeDigits   = 4
	maxCodeTries = 5 // the sixth try is locked, also with the right code (BIZ-11)
	// codeKeyVersion is the encryption ring version for new codes.
	codeKeyVersion byte = 1
)

var codeRe = regexp.MustCompile(`^[0-9]{4}$`)

// codeAAD ties a sealed code to its order, so a value copied to another row
// cannot be opened there.
func codeAAD(orderID uuid.UUID) []byte { return []byte("delivery_code:" + orderID.String()) }

func (s *Service) codeRing(ctx context.Context) (*crypt.Ring, error) {
	if s.secrets == nil {
		return nil, errors.New("order: no secret store for delivery codes")
	}
	return secrets.Ring(ctx, s.secrets, codeKeyVersion)
}

// issueCode makes a new random code for the order, stores it sealed (AES-256-GCM)
// and resets the tries. It is the side effect of T15 and T18 (FR-DLV-03). The
// code goes to the customer only: no log, event or response to anyone else holds it.
func (s *Service) issueCode(ctx context.Context, tx *db.Tx, orderID uuid.UUID) error {
	ring, err := s.codeRing(ctx)
	if err != nil {
		return err
	}
	sealed := ring.Seal([]byte(crypt.RandomDigits(codeDigits)), codeAAD(orderID))
	_, err = tx.Exec(ctx, `UPDATE orders SET delivery_code_enc = $2, delivery_code_attempts = 0 WHERE id = $1`, orderID, sealed)
	return err
}

// codeOf reads the order's code and the tries used. found is false until one was made.
func (s *Service) codeOf(ctx context.Context, tx *db.Tx, orderID uuid.UUID) (code string, tries int, found bool, err error) {
	var sealed []byte
	if err = tx.QueryRow(ctx, `SELECT delivery_code_enc, delivery_code_attempts FROM orders WHERE id = $1`, orderID).Scan(&sealed, &tries); err != nil || sealed == nil {
		return "", tries, false, db.IgnoreNoRows(err)
	}
	ring, err := s.codeRing(ctx)
	if err != nil {
		return "", tries, false, err
	}
	plain, err := ring.Open(sealed, codeAAD(orderID))
	return string(plain), tries, err == nil, err
}

// CompleteInput is the body of POST /driver/deliveries/{id}/complete.
type CompleteInput struct {
	Version int    `json:"version"`
	Code    string `json:"code"`
}

// Complete is T16: the driver types the code the customer tells them. The code is
// compared in constant time. A wrong code counts as a try and the count is saved
// even though the answer is an error; after five wrong tries the order is locked
// until the admin unlocks it (BIZ-11).
//
// It runs as the system in the database because T16 writes the ledger, which no
// driver may. The driver is checked in code instead: the order must be theirs
// (else NOT_FOUND, AUTHZ-03), and the move itself still goes through transition.
func (s *Service) Complete(ctx context.Context, scope db.Scope, orderID uuid.UUID, in CompleteInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Check("code", codeRe.MatchString(in.Code), "invalid").Err(); err != nil {
		return Detail{}, err
	}
	var out Detail
	wrong := false
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		wrong = false
		o, found, err := lockOrder(ctx, tx, orderID)
		if err != nil || !found || !inScope(scope, o) {
			return apperr.NotFoundUnless(err, found && inScope(scope, o))
		}
		switch {
		case o.Version != in.Version:
			return apperr.New(apperr.StaleOrder)
		case o.Status != enum.OrderStatusOutForDelivery:
			return apperr.New(apperr.InvalidTransition)
		}
		code, tries, ok, err := s.codeOf(ctx, tx, orderID)
		switch {
		case err != nil:
			return err
		case tries >= maxCodeTries:
			return apperr.New(apperr.DeliveryCodeLocked)
		case !ok || !crypt.Equal([]byte(in.Code), []byte(code)):
			wrong = true
			_, err = tx.Exec(ctx, `UPDATE orders SET delivery_code_attempts = delivery_code_attempts + 1 WHERE id = $1`, orderID)
			return err // nil commits the count
		}
		if err := s.transition(ctx, tx, scope, orderID, enum.OrderStatusDelivered, TransitionInput{Version: o.Version, verified: true}); err != nil {
			return err
		}
		out, err = s.detail(ctx, tx, scope, orderID)
		return err
	})
	if err == nil && wrong {
		return Detail{}, apperr.New(apperr.DeliveryCodeInvalid)
	}
	return out, err
}

// FailInput is the body of POST /driver/deliveries/{id}/fail.
type FailInput struct {
	Version int    `json:"version"`
	Reason  string `json:"reason"`
}

// Fail is T17: the driver could not deliver, and says why.
func (s *Service) Fail(ctx context.Context, scope db.Scope, orderID uuid.UUID, in FailInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Text("reason", in.Reason, 3, maxReason).Err(); err != nil {
		return Detail{}, err
	}
	return s.Transition(ctx, scope, orderID, enum.OrderStatusDeliveryFailed, TransitionInput{Version: in.Version, Reason: in.Reason})
}

// UnlockCode gives a locked order its five tries back (FR-ADM-03). Only the
// admin does it, and the order stays out for delivery; the code is the same.
func (s *Service) UnlockCode(ctx context.Context, scope db.Scope, orderID uuid.UUID) (Detail, error) {
	var out Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		o, found, err := lockOrder(ctx, tx, orderID)
		if err != nil || !found || !inScope(scope, o) {
			return apperr.NotFoundUnless(err, found && inScope(scope, o))
		}
		if o.Status != enum.OrderStatusOutForDelivery {
			return apperr.New(apperr.InvalidTransition)
		}
		var tries int
		if err := tx.QueryRow(ctx, `SELECT delivery_code_attempts FROM orders WHERE id = $1`, orderID).Scan(&tries); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET delivery_code_attempts = 0 WHERE id = $1`, orderID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{Action: "order.unlock_code", Entity: "order", EntityID: orderID.String(), ShopID: o.ShopID,
			Before: map[string]any{"attempts": tries}, After: map[string]any{"attempts": 0}}); err != nil {
			return err
		}
		out, err = s.detail(ctx, tx, scope, orderID)
		return err
	})
	return out, err
}
