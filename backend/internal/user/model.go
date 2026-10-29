// Package user is the signed-in customer's own data: name, verified phone and
// delivery addresses. Everything here runs in the caller's own scope; sign-in
// and sessions live in auth.
package user

import (
	"github.com/google/uuid"

	"gp/internal/delivery"
)

// Address is a saved delivery place (FR-ADDR-01).
type Address struct {
	ID        uuid.UUID `json:"id"`
	Label     string    `json:"label"`
	Line1     string    `json:"line1"`
	Landmark  *string   `json:"landmark"`
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	IsDefault bool      `json:"isDefault"`
}

// AddressInput creates an address.
type AddressInput struct {
	Label     string  `json:"label"`
	Line1     string  `json:"line1"`
	Landmark  *string `json:"landmark"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	IsDefault bool    `json:"isDefault"`
}

// AddressPatch changes some fields. An empty landmark clears it.
type AddressPatch struct {
	Label     *string  `json:"label"`
	Line1     *string  `json:"line1"`
	Landmark  *string  `json:"landmark"`
	Lat       *float64 `json:"lat"`
	Lng       *float64 `json:"lng"`
	IsDefault *bool    `json:"isDefault"`
}

// AddressResult is an address with the shops that deliver to its pin (FR-ADDR-02).
type AddressResult struct {
	Address         Address              `json:"address"`
	DeliveringShops []delivery.Reachable `json:"deliveringShops"`
}

// PhoneCodeInput is a request to send a verification code.
type PhoneCodeInput struct {
	Phone          string `json:"phone"`
	TurnstileToken string `json:"turnstileToken"`
}
