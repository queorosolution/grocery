package order

import (
	"strconv"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
)

// maxChanges is the most lines one revision may touch; a shop's own line limit is lower.
const maxChanges = 100

// Revision is what the shop changes when it revises a bill (docs/06 section 8).
type Revision struct {
	Changes []Change   `json:"changes"`
	SlotID  *uuid.UUID `json:"slotId"`
}

// Change is one edit of one line of the order: a new quantity, a removal or a
// substitute. Exactly one of the three is set.
type Change struct {
	ItemID     uuid.UUID   `json:"itemId"`
	Qty        *int        `json:"qty"`
	Remove     bool        `json:"remove"`
	Substitute *Substitute `json:"substitute"`
}

// Substitute is another product of the same shop that takes the place of a line.
// The server prices it.
type Substitute struct {
	ShopProductID uuid.UUID `json:"shopProductId"`
	Qty           int       `json:"qty"`
}

// check validates the shape of the request. What it means for this order is
// checked when it is applied.
func (r Revision) check() error {
	fields := map[string]string{}
	switch n := len(r.Changes); {
	case n == 0:
		fields["changes"] = "required"
	case n > maxChanges:
		fields["changes"] = "too_many"
	}
	seen := map[uuid.UUID]bool{}
	for i, c := range r.Changes {
		key := "changes." + strconv.Itoa(i)
		kinds := 0
		if c.Qty != nil {
			kinds++
			if *c.Qty < 1 || *c.Qty > absoluteMaxQty {
				fields[key+".qty"] = "out_of_range"
			}
		}
		if c.Remove {
			kinds++
		}
		if c.Substitute != nil {
			kinds++
			if c.Substitute.ShopProductID == uuid.Nil {
				fields[key+".substitute.shopProductId"] = "required"
			}
			if c.Substitute.Qty < 1 || c.Substitute.Qty > absoluteMaxQty {
				fields[key+".substitute.qty"] = "out_of_range"
			}
		}
		switch {
		case c.ItemID == uuid.Nil:
			fields[key+".itemId"] = "required"
		case seen[c.ItemID]:
			fields[key+".itemId"] = "duplicate"
		case kinds != 1:
			fields[key] = "one_change_only"
		}
		seen[c.ItemID] = true
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// absoluteMaxQty is the same ceiling billing.Build applies to any quantity.
const absoluteMaxQty = 999

// liveLine is a line of the current revision that still counts.
type liveLine struct {
	ID            uuid.UUID
	ShopProductID uuid.UUID
	Qty           int
	SubPref       enum.SubstitutionPref
}

// plan is what a revision does to the current lines: which are kept (and with
// what quantity), which are removed, and which new product replaces which line.
type plan struct {
	inputs      []billing.LineInput
	removed     []uuid.UUID             // current lines that end as REMOVED
	replacement map[uuid.UUID]uuid.UUID // substitute shop product -> the line it replaces
	changed     bool
}

// planOf applies the changes to the live lines. A change for a line the order
// does not have, or no real change at all, is a bad request.
func planOf(live []liveLine, changes []Change) (plan, error) {
	byID := map[uuid.UUID]Change{}
	for _, c := range changes {
		byID[c.ItemID] = c
	}
	p := plan{replacement: map[uuid.UUID]uuid.UUID{}}
	used := 0
	for _, l := range live {
		c, has := byID[l.ID]
		if has {
			used++
		}
		switch {
		case has && c.Remove:
			p.removed, p.changed = append(p.removed, l.ID), true
		case has && c.Substitute != nil:
			p.removed, p.changed = append(p.removed, l.ID), true
			p.inputs = append(p.inputs, billing.LineInput{ShopProductID: c.Substitute.ShopProductID, Qty: c.Substitute.Qty, SubPref: l.SubPref})
			p.replacement[c.Substitute.ShopProductID] = l.ID
		case has && c.Qty != nil:
			p.changed = p.changed || *c.Qty != l.Qty
			p.inputs = append(p.inputs, billing.LineInput{ShopProductID: l.ShopProductID, Qty: *c.Qty, SubPref: l.SubPref})
		default:
			p.inputs = append(p.inputs, billing.LineInput{ShopProductID: l.ShopProductID, Qty: l.Qty, SubPref: l.SubPref})
		}
	}
	switch {
	case used != len(changes):
		return plan{}, apperr.Validation(map[string]string{"changes": "unknown_item"})
	case !p.changed:
		return plan{}, apperr.Validation(map[string]string{"changes": "no_change"})
	case len(p.inputs) == 0:
		return plan{}, apperr.Validation(map[string]string{"changes": "removes_all"})
	}
	return p, nil
}
