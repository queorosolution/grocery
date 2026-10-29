package order

import (
	"context"
	"regexp"
	"strconv"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/delivery"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/user"
)

// maxRequestLines is a hard stop on the size of a request, far above any shop's
// own max_lines. The shop's limit is checked by the bill builder.
const maxRequestLines = 200

// cartIDPattern is what the cart service issues; anything else is refused before it is sent on.
var cartIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// QuoteInput is the body of POST /bills/quote. Quantities are not in it: they
// come from the cart, and the prices from our own database.
type QuoteInput struct {
	ShopID    uuid.UUID `json:"shopId"`
	CartID    string    `json:"cartId"`
	AddressID uuid.UUID `json:"addressId"`
	// Destination is a one-time place instead of a saved address: send one of the two.
	Destination *user.Destination `json:"destination"`
	SlotID      *uuid.UUID        `json:"slotId"`
	Lines       []QuoteRef        `json:"lines"`
}

// QuoteRef is the customer's choice for one cart line: what to do when it is not available.
type QuoteRef struct {
	ItemID  uuid.UUID             `json:"itemId"`
	SubPref enum.SubstitutionPref `json:"subPref"`
}

// where is the place the order goes to: the destination, or the saved address.
// A destination was checked by validate, so only the address read can fail.
func (s *Service) where(ctx context.Context, tx *db.Tx, userID uuid.UUID, in QuoteInput) (user.Address, error) {
	if in.Destination != nil {
		return in.Destination.Address()
	}
	return s.users.AddressFor(ctx, tx, userID, in.AddressID)
}

func (in QuoteInput) validate() error {
	f := map[string]string{}
	if in.ShopID == uuid.Nil {
		f["shopId"] = "required"
	}
	if !cartIDPattern.MatchString(in.CartID) {
		f["cartId"] = "invalid"
	}
	switch {
	case in.AddressID != uuid.Nil && in.Destination != nil:
		f["destination"] = "not_with_address"
	case in.AddressID == uuid.Nil && in.Destination == nil:
		f["addressId"] = "required"
	case in.Destination != nil:
		if _, err := in.Destination.Address(); err != nil {
			return err
		}
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
