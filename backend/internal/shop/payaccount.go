package shop

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// PayAccount is a payment account as the payment module uses it. Unlike Account
// it carries the secret's name in the secret store (never the secret). It is not
// an API type: nothing sends it to a client.
type PayAccount struct {
	ID         uuid.UUID
	Provider   enum.PaymentProvider
	Mode       enum.PaymentMode
	MerchantID string
	SecretRef  string
	Bank       *Bank
}

const payAccountColumns = `pa.id, pa.provider, pa.mode, COALESCE(pa.merchant_id, ''), COALESCE(pa.secret_ref, ''), pa.public_info->'bank'`

func scanPayAccount(row interface{ Scan(...any) error }) (PayAccount, error) {
	var a PayAccount
	var bank []byte
	if err := row.Scan(&a.ID, &a.Provider, &a.Mode, &a.MerchantID, &a.SecretRef, &bank); err != nil {
		return a, err
	}
	if bank != nil {
		a.Bank = new(Bank)
		return a, json.Unmarshal(bank, a.Bank)
	}
	return a, nil
}

// PayAccountOf is the account a customer of this shop pays into: the ACTIVE one
// the shop's settings name, of the provider they name. A shop without one, or a
// MANUAL one while the feature is off, is PAYMENT_NOT_AVAILABLE. Payment accounts
// are for the system scope only.
func (s *Service) PayAccountOf(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (PayAccount, error) {
	a, err := scanPayAccount(tx.QueryRow(ctx, `
		SELECT `+payAccountColumns+`
		FROM shop_settings ss JOIN payment_accounts pa ON pa.id = ss.payment_account_id
		WHERE ss.shop_id = $1 AND pa.status = 'ACTIVE' AND pa.provider = ss.payment_provider
		  AND (pa.owner_type = 'PLATFORM' OR pa.shop_id = ss.shop_id)`, shopID))
	if err = db.IgnoreNoRows(err); err != nil || a.ID == uuid.Nil || (a.Provider == enum.PaymentProviderManual && !s.manualEnabled) {
		return PayAccount{}, orUnavailable(err)
	}
	return a, nil
}

// PayAccountByID reads one account by id, whatever its status: a payment that
// started before the account was disabled still has to be verified.
func (s *Service) PayAccountByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (PayAccount, bool, error) {
	a, err := scanPayAccount(tx.QueryRow(ctx, `SELECT `+payAccountColumns+` FROM payment_accounts pa WHERE pa.id = $1`, id))
	return a, err == nil, db.IgnoreNoRows(err)
}

func orUnavailable(err error) error {
	if err != nil {
		return err
	}
	return apperr.New(apperr.PaymentNotAvailable)
}
