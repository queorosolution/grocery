package shop

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

var merchantIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

const (
	minSecret, maxSecret = 8, 200
	secretPrefix         = "payhere/" // SSM name: payhere/<accountId> (docs/13 §2)
)

// Account is a payment account as the admin sees it. The merchant secret is
// write-only: only HasSecret says that one is stored.
type Account struct {
	ID         uuid.UUID                 `json:"id"`
	OwnerType  enum.PaymentAccountOwner  `json:"ownerType"`
	ShopID     *uuid.UUID                `json:"shopId"`
	Provider   enum.PaymentProvider      `json:"provider"`
	Mode       enum.PaymentMode          `json:"mode"`
	MerchantID *string                   `json:"merchantId"`
	Status     enum.PaymentAccountStatus `json:"status"`
	HasSecret  bool                      `json:"hasSecret"`
	Bank       *Bank                     `json:"bank"`
	CreatedAt  time.Time                 `json:"createdAt"`
	UpdatedAt  time.Time                 `json:"updatedAt"`
}

// Bank is where a customer sends money for a MANUAL account. It is shown to the
// customer who pays by bank transfer, so it is not a secret.
type Bank struct {
	BankName      string `json:"bankName"`
	AccountName   string `json:"accountName"`
	AccountNumber string `json:"accountNumber"`
	Branch        string `json:"branch"`
}

const maxBankField = 80

func (b Bank) validate(v *validate.Validator) {
	for field, val := range map[string]string{"bank.bankName": b.BankName, "bank.accountName": b.AccountName, "bank.accountNumber": b.AccountNumber} {
		n := utf8.RuneCountInString(strings.TrimSpace(val))
		v.Check(field, n >= 1 && n <= maxBankField && val == strings.TrimSpace(val), "invalid")
	}
	v.Check("bank.branch", utf8.RuneCountInString(b.Branch) <= maxBankField && b.Branch == strings.TrimSpace(b.Branch), "invalid")
}

// CreateAccountInput is the create request. MerchantSecret never leaves this
// struct: it goes to the secret store and is not echoed, logged or audited.
type CreateAccountInput struct {
	OwnerType      enum.PaymentAccountOwner `json:"ownerType"`
	ShopID         *uuid.UUID               `json:"shopId"`
	Provider       enum.PaymentProvider     `json:"provider"`
	Mode           enum.PaymentMode         `json:"mode"`
	MerchantID     string                   `json:"merchantId"`
	MerchantSecret string                   `json:"merchantSecret"`
	Bank           *Bank                    `json:"bank"`
}

// UpdateAccountInput changes only what is present.
type UpdateAccountInput struct {
	Mode           *enum.PaymentMode          `json:"mode"`
	MerchantID     *string                    `json:"merchantId"`
	MerchantSecret *string                    `json:"merchantSecret"`
	Status         *enum.PaymentAccountStatus `json:"status"`
	Bank           *Bank                      `json:"bank"`
}

func (c CreateAccountInput) validate() error {
	v := validate.New().
		Check("ownerType", c.OwnerType.Valid(), "invalid").
		Check("provider", c.Provider.Valid(), "invalid")
	if c.Mode != "" {
		v.Check("mode", c.Mode.Valid(), "invalid")
	}
	switch c.OwnerType {
	case enum.PaymentAccountOwnerPlatform:
		v.Check("shopId", c.ShopID == nil, "not_allowed")
	case enum.PaymentAccountOwnerShop:
		v.Check("shopId", c.ShopID != nil, "required")
	}
	switch c.Provider {
	case enum.PaymentProviderPayhere:
		checkMerchant(v, c.MerchantID, c.MerchantSecret)
		v.Check("bank", c.Bank == nil, "not_allowed")
	case enum.PaymentProviderManual:
		v.Check("merchantId", c.MerchantID == "", "not_allowed").Check("merchantSecret", c.MerchantSecret == "", "not_allowed")
		if c.Bank != nil {
			c.Bank.validate(v)
		}
	}
	return v.Err()
}

func checkMerchant(v *validate.Validator, id, secret string) {
	v.Check("merchantId", merchantIDRe.MatchString(id), "invalid")
	checkSecret(v, secret)
}

// checkSecret asks for printable ASCII without spaces: PayHere secrets are.
func checkSecret(v *validate.Validator, secret string) {
	ok := utf8.RuneCountInString(secret) >= minSecret && len(secret) <= maxSecret
	for i := 0; ok && i < len(secret); i++ {
		ok = secret[i] > ' ' && secret[i] < 0x7f
	}
	v.Check("merchantSecret", ok, "invalid")
}

func (s *Service) Accounts(ctx context.Context, scope db.Scope) ([]Account, error) {
	var out []Account
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = listAccounts(ctx, tx)
		return err
	})
	return out, err
}

// CreateAccount adds an account. A PayHere account is stored with its secret in
// one go: the secret is written to the secret store before the row commits, so
// a failed write leaves no half-connected account behind.
func (s *Service) CreateAccount(ctx context.Context, scope db.Scope, in CreateAccountInput) (Account, error) {
	if in.Mode == "" {
		in.Mode = enum.PaymentModeSandbox
	}
	if err := in.validate(); err != nil {
		return Account{}, err
	}
	id := uuid.New()
	var acc Account
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if in.ShopID != nil {
			_, found, err := shopByID(ctx, tx, *in.ShopID, false)
			if err != nil {
				return err
			}
			if !found {
				return validate.New().Check("shopId", false, "unknown").Err()
			}
		}
		var ref *string
		var merchant *string
		if in.Provider == enum.PaymentProviderPayhere {
			name := secretPrefix + id.String()
			ref, merchant = &name, &in.MerchantID
			if err := s.secrets.Put(ctx, name, in.MerchantSecret); err != nil {
				return apperr.New(apperr.Internal).Wrap(err)
			}
		}
		var err error
		acc, err = insertAccount(ctx, tx, id, in, ref, merchant)
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "payment_account.create", Entity: "payment_account", EntityID: id.String(), ShopID: shopOf(acc.ShopID),
			After: acc,
		})
	})
	return acc, err
}

// UpdateAccount changes mode, merchant id, secret or status. Rotating the secret
// replaces it in the secret store; the audit row records that it was rotated,
// never the value.
func (s *Service) UpdateAccount(ctx context.Context, scope db.Scope, id uuid.UUID, in UpdateAccountInput) (Account, error) {
	var out Account
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, ref, found, err := accountForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.NotFound)
		}
		next, newRef, err := applyAccount(before, ref, in)
		if err != nil {
			return err
		}
		if in.MerchantSecret != nil {
			if err := s.secrets.Put(ctx, *newRef, *in.MerchantSecret); err != nil {
				return apperr.New(apperr.Internal).Wrap(err)
			}
		}
		if out, err = saveAccount(ctx, tx, next, newRef); err != nil {
			return err
		}
		after := struct {
			Account
			SecretRotated bool `json:"secretRotated"`
		}{out, in.MerchantSecret != nil}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "payment_account.update", Entity: "payment_account", EntityID: id.String(), ShopID: shopOf(out.ShopID),
			Before: before, After: after,
		})
	})
	return out, err
}

// applyAccount lays the update over the stored account and checks the result
// can stand: a PayHere account is ACTIVE only with a merchant id and a secret.
func applyAccount(a Account, ref *string, in UpdateAccountInput) (Account, *string, error) {
	v := validate.New()
	if in.Mode != nil {
		v.Check("mode", in.Mode.Valid(), "invalid")
		a.Mode = *in.Mode
	}
	if in.Status != nil {
		v.Check("status", *in.Status == enum.PaymentAccountStatusActive || *in.Status == enum.PaymentAccountStatusDisabled, "invalid")
		a.Status = *in.Status
	}
	if a.Provider == enum.PaymentProviderManual {
		v.Check("merchantId", in.MerchantID == nil, "not_allowed").Check("merchantSecret", in.MerchantSecret == nil, "not_allowed")
		if in.Bank != nil {
			in.Bank.validate(v)
			a.Bank = in.Bank
		}
	} else {
		v.Check("bank", in.Bank == nil, "not_allowed")
		if in.MerchantID != nil {
			v.Check("merchantId", merchantIDRe.MatchString(*in.MerchantID), "invalid")
			a.MerchantID = in.MerchantID
		}
		if in.MerchantSecret != nil {
			checkSecret(v, *in.MerchantSecret)
			name := secretPrefix + a.ID.String()
			ref = &name
			a.HasSecret = true
		}
		v.Check("status", a.Status != enum.PaymentAccountStatusActive || (a.MerchantID != nil && a.HasSecret), "needs_credentials")
	}
	return a, ref, v.Err()
}

func shopOf(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
