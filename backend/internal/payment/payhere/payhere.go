// Package payhere is the PayHere checkout provider (docs/09 section 2). The hash
// and the notify checks follow PayHere's Checkout API. Re-check their docs before go-live.
package payhere

import (
	"context"
	"crypto/md5" // #nosec G501 -- PayHere's documented signature scheme is MD5; it is not used to protect anything of ours
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gp/internal/order"
	"gp/internal/payment"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/secrets"
	"gp/internal/shop"
)

const (
	// Currency is the only one PayHere is asked to charge in.
	Currency = "LKR"

	sandboxAction = "https://sandbox.payhere.lk/pay/checkout"
	liveAction    = "https://www.payhere.lk/pay/checkout"
)

// Provider builds checkouts and checks notifies. The merchant secret is read
// from the secret store for each call and never kept.
type Provider struct {
	secrets   secrets.Getter
	webOrigin string // return and cancel links
	notifyURL string
}

func New(sec secrets.Getter, webOrigin, notifyURL string) *Provider {
	return &Provider{secrets: sec, webOrigin: strings.TrimRight(webOrigin, "/"), notifyURL: notifyURL}
}

// Checkout is what the browser posts to PayHere.
type Checkout struct {
	Provider enum.PaymentProvider `json:"provider"`
	Action   string               `json:"action"`
	Fields   map[string]string    `json:"fields"`
}

// Checkout builds the signed form (docs/09 section 2.2). order_id is our payment
// id, so the callback can find the attempt. The hash covers the amount, so a
// customer who edits it in the browser pays nothing.
func (p *Provider) Checkout(ctx context.Context, acc shop.PayAccount, pay payment.Payment, o order.Payable) (any, error) {
	secret, err := p.secrets.Get(ctx, acc.SecretRef)
	if err != nil {
		return nil, fmt.Errorf("payhere: merchant secret: %w", err)
	}
	amount := money.Decimal(pay.AmountCents)
	first, last := splitName(o.Customer.Name)
	f := map[string]string{
		"merchant_id": acc.MerchantID,
		"return_url":  p.webOrigin + "/order/" + o.ID.String() + "?paid=1",
		"cancel_url":  p.webOrigin + "/order/" + o.ID.String(),
		"notify_url":  p.notifyURL,
		"order_id":    pay.ID.String(),
		"items":       "Order " + o.Code,
		"currency":    Currency,
		"amount":      amount,
		"first_name":  first,
		"last_name":   last,
		"email":       o.Customer.Email,
		"phone":       o.Customer.Phone,
		"address":     o.Address,
		"city":        "-", // an address has no city field yet; PayHere only wants it filled
		"country":     "Sri Lanka",
		"hash":        CheckoutHash(acc.MerchantID, pay.ID.String(), amount, Currency, secret),
	}
	action := liveAction
	if acc.Mode == enum.PaymentModeSandbox {
		action = sandboxAction
	}
	return Checkout{Provider: enum.PaymentProviderPayhere, Action: action, Fields: f}, nil
}

// CheckoutHash is UPPER(MD5(merchant_id + order_id + amount + currency + UPPER(MD5(secret)))).
func CheckoutHash(merchantID, orderID, amount, currency, secret string) string {
	return upperMD5(merchantID + orderID + amount + currency + upperMD5(secret))
}

// NotifySignature is the md5sig of a callback: the checkout hash with the status
// code before the secret. The fake PayHere tool signs with it too.
func NotifySignature(merchantID, orderID, amount, currency, statusCode, secret string) string {
	return upperMD5(merchantID + orderID + amount + currency + statusCode + upperMD5(secret))
}

func upperMD5(s string) string {
	sum := md5.Sum([]byte(s)) // #nosec G401 -- see the import
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// splitName gives PayHere the two names it asks for. A single name is the first
// name and the last is a dash.
func splitName(name string) (first, last string) {
	first, last, _ = strings.Cut(strings.TrimSpace(name), " ")
	if last = strings.TrimSpace(last); last == "" {
		last = "-"
	}
	return first, last
}

// Fields every callback must carry (docs/09 section 2.3). The card fields are
// not in this list on purpose: nothing of them is kept.
var kept = []string{"merchant_id", "order_id", "payment_id", "payhere_amount", "payhere_currency", "status_code", "custom_1", "custom_2", "method", "status_message"}

// ParseNotify runs checks 4 to 7 of docs/09 section 2.3: the signature (constant
// time), then the merchant, the currency and the amount. A wrong signature is
// ErrBadSignature; a right one that does not match the payment is ErrMismatch.
func (p *Provider) ParseNotify(ctx context.Context, acc shop.PayAccount, pay payment.Payment, form url.Values) (payment.Notified, error) {
	secret, err := p.secrets.Get(ctx, acc.SecretRef)
	if err != nil {
		return payment.Notified{}, fmt.Errorf("payhere: merchant secret: %w", err)
	}
	g := form.Get
	want := NotifySignature(g("merchant_id"), g("order_id"), g("payhere_amount"), g("payhere_currency"), g("status_code"), secret)
	if subtle.ConstantTimeCompare([]byte(strings.ToUpper(g("md5sig"))), []byte(want)) != 1 {
		return payment.Notified{}, payment.ErrBadSignature
	}
	paid, ok := money.ParseDecimal(g("payhere_amount"))
	code, codeErr := strconv.Atoi(g("status_code"))
	if g("merchant_id") != acc.MerchantID || g("payhere_currency") != Currency || !ok || paid != pay.AmountCents || codeErr != nil {
		return payment.Notified{}, payment.ErrMismatch
	}
	raw := make(map[string]string, len(kept))
	for _, k := range kept {
		if v := g(k); v != "" {
			raw[k] = v
		}
	}
	return payment.Notified{Ref: g("payment_id"), Code: code, Method: g("method"), Raw: raw}, nil
}
