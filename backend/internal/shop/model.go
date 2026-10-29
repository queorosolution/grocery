// Package shop owns shops, their settings, weekly hours and payment accounts
// (docs/06 §10, docs/02 F3). Only the platform admin changes any of it; the
// public side reads a small list for the site build.
package shop

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
)

// Shop is the profile row.
type Shop struct {
	ID              uuid.UUID       `json:"id"`
	Slug            string          `json:"slug"`
	Name            string          `json:"name"`
	Phone           *string         `json:"phone"`
	AddressText     *string         `json:"addressText"`
	Lat             float64         `json:"lat"`
	Lng             float64         `json:"lng"`
	LogoURL         *string         `json:"logoUrl"`
	ThemeColor      string          `json:"themeColor"`
	Status          enum.ShopStatus `json:"status"`
	AcceptingOrders bool            `json:"acceptingOrders"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// Settings are the per-shop options. The cart fields are read-only here: they
// are filled when the admin creates the cart tenant (P15).
type Settings struct {
	DeliveryType           enum.DeliveryType    `json:"deliveryType"`
	PaymentProvider        enum.PaymentProvider `json:"paymentProvider"`
	PaymentAccountID       *uuid.UUID           `json:"paymentAccountId"`
	DriverSource           enum.DriverSource    `json:"driverSource"`
	NotifyChannels         []enum.NotifyChannel `json:"notifyChannels"`
	MarkupBP               int                  `json:"markupBp"`
	MinPlatformMarginCents int64                `json:"minPlatformMarginCents"`
	MinOrderCents          int64                `json:"minOrderCents"`
	MaxLines               int                  `json:"maxLines"`
	ReviewTimeoutMin       int                  `json:"reviewTimeoutMin"`
	RevisionTimeoutMin     int                  `json:"revisionTimeoutMin"`
	PaymentTimeoutMin      int                  `json:"paymentTimeoutMin"`
}

// SettingsView adds the read-only cart fields.
type SettingsView struct {
	Settings
	CartTenantID  *string `json:"cartTenantId"`
	CartPublicKey *string `json:"cartPublicKey"`
}

// Day is one open day. Opens and Closes are "HH:MM" in Asia/Colombo; a shop that
// closes at midnight writes "24:00".
type Day struct {
	Weekday int    `json:"weekday"` // ISO: 1 = Monday ... 7 = Sunday
	Opens   string `json:"opens"`
	Closes  string `json:"closes"`
}

// Detail is the admin's whole picture of one shop.
type Detail struct {
	Shop      Shop         `json:"shop"`
	Settings  SettingsView `json:"settings"`
	Hours     []Day        `json:"hours"`
	Readiness Readiness    `json:"readiness"`
}

// PublicShop is what the site build needs.
type PublicShop struct {
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	LogoURL   *string   `json:"logoUrl"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateInput and UpdateInput are the validated profile requests.
type CreateInput struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Phone       *string `json:"phone"`
	AddressText *string `json:"addressText"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	LogoURL     *string `json:"logoUrl"`
	ThemeColor  *string `json:"themeColor"`
}

// UpdateInput changes only the fields that are present.
type UpdateInput struct {
	Slug        *string          `json:"slug"`
	Name        *string          `json:"name"`
	Phone       *string          `json:"phone"`
	AddressText *string          `json:"addressText"`
	Lat         *float64         `json:"lat"`
	Lng         *float64         `json:"lng"`
	LogoURL     *string          `json:"logoUrl"`
	ThemeColor  *string          `json:"themeColor"`
	Status      *enum.ShopStatus `json:"status"`
}
