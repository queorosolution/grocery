package connector

import (
	"strings"
	"unicode/utf8"

	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// Item is one source row after cleaning. Cleaning never fails the whole run:
// a row that cannot be used is reported as Invalid, a bad price is kept as
// PriceBad so the plan can hold it with the old price in place.
type Item struct {
	ExternalID string
	Barcode    *string
	Name       string
	Brand      *string
	MRPCents   *int64
	PriceCents int64 // 0 when PriceBad
	PriceBad   bool
	Stock      *int64
	Available  *bool
	Unit       string // PCS, KG, G, L or ML
	WeightG    *int
	Category   *string
	ImageURL   *string
}

// Invalid is why a row was skipped.
type Invalid string

const (
	NoExternalID Invalid = "no_external_id"
	NoName       Invalid = "no_name"
)

const (
	maxExternalID = 100 // shop_products.external_id
	maxName       = 120 // products.name
	maxBrand      = 60
	maxCategory   = 60
)

// Normalise cleans one row. raw is keyed by our field names (see Fields);
// missing and NULL values are absent or nil. Money is read in the connector's
// unit, the barcode keeps its digits only, the unit is one of ours.
func Normalise(raw map[string]*string, unit enum.MoneyUnit, imgBase string) (Item, Invalid) {
	var it Item
	it.ExternalID = text(raw["externalId"])
	if it.ExternalID == "" || utf8.RuneCountInString(it.ExternalID) > maxExternalID {
		return Item{}, NoExternalID
	}
	it.Name = truncate(text(raw["name"]), maxName)
	if utf8.RuneCountInString(it.Name) < 2 {
		return Item{ExternalID: it.ExternalID}, NoName
	}
	if p := raw["price"]; p != nil {
		cents, ok := ParseMoney(*p, unit)
		it.PriceCents, it.PriceBad = cents, !ok || cents > maxShopPrice
		if it.PriceBad {
			it.PriceCents = 0
		}
	} else {
		it.PriceBad = true
	}
	if m := raw["mrp"]; m != nil {
		if cents, ok := ParseMoney(*m, unit); ok && cents > 0 && cents <= maxShopPrice {
			it.MRPCents = &cents
		}
	}
	it.Barcode = barcode(raw["barcode"])
	it.Brand = optText(raw["brand"], maxBrand)
	it.Category = optText(raw["category"], maxCategory)
	it.Stock = wholeNumber(raw["stock"])
	it.Available = boolean(raw["available"])
	it.Unit = unitOf(raw["unit"])
	if w := wholeNumber(raw["weightG"]); w != nil && *w >= 1 && *w <= 100000 {
		g := int(*w)
		it.WeightG = &g
	}
	// Only images that live under our own image host are kept; the catalog does the same.
	if u := optText(raw["imageUrl"], 300); u != nil && imgBase != "" && validate.New().HTTPSURLUnder("u", *u, imgBase, 300).Err() == nil {
		it.ImageURL = u
	}
	return it, ""
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func optText(s *string, max int) *string {
	if t := truncate(text(s), max); t != "" {
		return &t
	}
	return nil
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// barcode keeps digits when 6 to 14 are left (the catalog's rule), else nothing.
func barcode(s *string) *string {
	var b strings.Builder
	for _, r := range text(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
		default:
			return nil
		}
	}
	if n := b.Len(); n < 6 || n > 14 {
		return nil
	}
	out := b.String()
	return &out
}

func boolean(s *string) *bool {
	var v bool
	switch strings.ToLower(text(s)) {
	case "t", "true", "1", "y", "yes":
		v = true
	case "f", "false", "0", "n", "no":
		v = false
	default:
		return nil
	}
	return &v
}

// wholeNumber reads a count such as stock: "12", "12.000" and "12.7" give 12.
// A value that is not a number is "not tracked".
func wholeNumber(s *string) *int64 {
	t := text(s)
	if t == "" {
		return nil
	}
	neg := strings.HasPrefix(t, "-")
	whole, _, _ := strings.Cut(strings.TrimPrefix(t, "-"), ".")
	n, ok := digits(whole)
	if !ok {
		return nil
	}
	if neg {
		n = -n
	}
	return &n
}

var units = map[string]string{
	"pcs": "PCS", "pc": "PCS", "piece": "PCS", "pieces": "PCS", "each": "PCS", "ea": "PCS", "nos": "PCS", "unit": "PCS",
	"kg": "KG", "kgs": "KG", "kilo": "KG", "kilogram": "KG", "kilograms": "KG",
	"g": "G", "gm": "G", "gms": "G", "gram": "G", "grams": "G",
	"l": "L", "lt": "L", "ltr": "L", "litre": "L", "liter": "L", "litres": "L", "liters": "L",
	"ml": "ML", "millilitre": "ML", "milliliter": "ML",
}

// unitOf maps the source's unit text to ours; anything unknown counts as pieces.
func unitOf(s *string) string {
	if u, ok := units[strings.ToLower(text(s))]; ok {
		return u
	}
	return "PCS"
}
