package connector

import "github.com/google/uuid"

// Action is what a sync does with one row.
type Action string

const (
	ActionUpdate    Action = "update"    // the shop product exists: change it
	ActionCreate    Action = "create"    // a new shop product: linked by barcode, or from a new master product
	ActionUnmatched Action = "unmatched" // nothing to attach it to: an issue for the admin
	ActionHold      Action = "hold"      // the price looks wrong: keep the old one and raise an issue
	ActionInvalid   Action = "invalid"   // the row has no usable id or name
)

// Reasons given with an action. They are words for the admin screen, not messages.
const (
	ReasonBarcode        = "barcode"         // create: linked to the master product with this barcode
	ReasonAutoCreate     = "auto_create"     // create: a new master product will be made
	ReasonNoBarcode      = "no_barcode"      // unmatched: nothing to match by
	ReasonUnknownBarcode = "unknown_barcode" // unmatched: no master product has this barcode
	ReasonAlreadyLinked  = "already_linked"  // unmatched: the shop sells that product under another id
	ReasonZeroPrice      = "zero_price"      // hold
	ReasonBadPrice       = "bad_price"       // hold: not a number, or above the limit
	ReasonOverMRP        = "over_mrp"        // hold
	ReasonBigDrop        = "big_drop"        // hold: more than 50% below the current price
)

// Existing is the shop product a row's external id already points to.
type Existing struct {
	ShopProductID  uuid.UUID
	ShopPriceCents int64
	MRPCents       *int64
}

// Match is what the database knows about a row.
type Match struct {
	Existing *Existing // by external id
	// ByBarcode is the master product with the row's barcode, if any.
	ByBarcode *uuid.UUID
	// BarcodeLinked is true when the shop already sells ByBarcode under another external id.
	BarcodeLinked bool
}

// Decision is the outcome for one row.
type Decision struct {
	Action Action
	Reason string
}

// Decide applies the matching and safety rules of docs/07 section 11 to one
// row. It is pure, so the preview and the run cannot disagree.
//
// Matching: the saved link (external id) first, then the barcode, then the
// connector's auto-create switch. Holds (FR-SYNC-10): a price of 0, a price
// above the MRP, or a drop of more than half the current price.
func Decide(it Item, m Match, autoCreate bool) Decision {
	var d Decision
	switch {
	case m.Existing != nil:
		d = Decision{Action: ActionUpdate}
	case m.ByBarcode != nil && m.BarcodeLinked:
		return Decision{Action: ActionUnmatched, Reason: ReasonAlreadyLinked}
	case m.ByBarcode != nil:
		d = Decision{Action: ActionCreate, Reason: ReasonBarcode}
	case autoCreate:
		d = Decision{Action: ActionCreate, Reason: ReasonAutoCreate}
	case it.Barcode == nil:
		return Decision{Action: ActionUnmatched, Reason: ReasonNoBarcode}
	default:
		return Decision{Action: ActionUnmatched, Reason: ReasonUnknownBarcode}
	}
	if reason := priceHold(it, m.Existing); reason != "" {
		return Decision{Action: ActionHold, Reason: reason}
	}
	return d
}

// priceHold returns why the new price must not be applied, or "".
func priceHold(it Item, existing *Existing) string {
	switch {
	case it.PriceBad:
		return ReasonBadPrice
	case it.PriceCents == 0:
		return ReasonZeroPrice
	}
	mrp := it.MRPCents
	if mrp == nil && existing != nil {
		mrp = existing.MRPCents
	}
	switch {
	case mrp != nil && it.PriceCents > *mrp:
		return ReasonOverMRP
	case existing != nil && it.PriceCents*2 < existing.ShopPriceCents:
		return ReasonBigDrop
	}
	return ""
}
