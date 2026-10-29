package order

import (
	"regexp"
	"strconv"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/delivery"
	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
)

// maxRequestLines is a hard stop on the size of a request, far above any shop's
// own max_lines. The shop's limit is checked by the bill builder.
const maxRequestLines = 200

// cartIDPattern is what the cart service issues; anything else is refused before it is sent on.
var cartIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// QuoteInput is the body of POST /bills/quote. Quantities are not in it: they
// come from the cart, and the prices from our own database.
type QuoteInput struct {
	ShopID    uuid.UUID  `json:"shopId"`
	CartID    string     `json:"cartId"`
	AddressID uuid.UUID  `json:"addressId"`
	SlotID    *uuid.UUID `json:"slotId"`
	Lines     []QuoteRef `json:"lines"`
}

// QuoteRef is the customer's choice for one cart line: what to do when it is not available.
type QuoteRef struct {
	ItemID  uuid.UUID             `json:"itemId"`
	SubPref enum.SubstitutionPref `json:"subPref"`
}

func (in QuoteInput) validate() error {
	f := map[string]string{}
	if in.ShopID == uuid.Nil {
		f["shopId"] = "required"
	}
	if !cartIDPattern.MatchString(in.CartID) {
		f["cartId"] = "invalid"
	}
	if in.AddressID == uuid.Nil {
		f["addressId"] = "required"
	}
	if in.SlotID != nil && *in.SlotID == uuid.Nil {
		f["slotId"] = "invalid"
	}
	if len(in.Lines) > maxRequestLines {
		f["lines"] = "too_many"
	}
	seen := map[uuid.UUID]bool{}
	for i, l := range in.Lines {
		switch {
		case l.ItemID == uuid.Nil:
			f["lines."+strconv.Itoa(i)+".itemId"] = "required"
		case seen[l.ItemID]:
			f["lines."+strconv.Itoa(i)+".itemId"] = "duplicate"
		}
		seen[l.ItemID] = true
	}
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// Quote is the bill as it would be now, plus the delivery windows to choose from.
type Quote struct {
	billing.Bill
	Slots []delivery.PublicSlot `json:"slots"`
}
