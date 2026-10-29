package pricing

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/validate"
)

// maxMarkups is more than the number of categories there will ever be.
const maxMarkups = 500

// Markup is the markup of one category in one shop. It replaces the shop's
// default markup for products of that category.
type Markup struct {
	CategoryID uuid.UUID `json:"categoryId"`
	MarkupBP   int       `json:"markupBp"`
}

func (s *Service) Markups(ctx context.Context, scope db.Scope, shopID uuid.UUID) ([]Markup, error) {
	var out []Markup
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := s.mustExist(ctx, tx, shopID); err != nil {
			return err
		}
		var err error
		out, err = markupsOf(ctx, tx, shopID)
		return err
	})
	return out, err
}

// SetMarkups replaces all category markups of a shop, then reprices the shop in
// the same transaction.
func (s *Service) SetMarkups(ctx context.Context, scope db.Scope, shopID uuid.UUID, in []Markup) ([]Markup, error) {
	if err := validateMarkups(in); err != nil {
		return nil, err
	}
	var out []Markup
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := s.mustExist(ctx, tx, shopID); err != nil {
			return err
		}
		before, err := markupsOf(ctx, tx, shopID)
		if err != nil {
			return err
		}
		if err := replaceMarkups(ctx, tx, shopID, in); err != nil {
			if db.IsForeignKeyViolation(err) {
				return validate.New().Check("markups", false, "unknown_category").Err()
			}
			return err
		}
		if out, err = markupsOf(ctx, tx, shopID); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{
			Action: "category_markups.set", Entity: "category_markups", EntityID: shopID.String(), ShopID: shopID,
			Before: before, After: out,
		}); err != nil {
			return err
		}
		return s.Reprice(ctx, tx, scope, shopID)
	})
	return out, err
}

func (s *Service) mustExist(ctx context.Context, tx *db.Tx, shopID uuid.UUID) error {
	ok, err := s.shops.Exists(ctx, tx, shopID)
	if err == nil && !ok {
		err = apperr.New(apperr.NotFound)
	}
	return err
}

func validateMarkups(in []Markup) error {
	v := validate.New().Check("markups", len(in) <= maxMarkups, "too_many")
	seen := make(map[uuid.UUID]bool, len(in))
	for i, m := range in {
		v.Int(fmt.Sprintf("markups[%d].markupBp", i), int64(m.MarkupBP), 0, MaxMarkupBP).
			Check(fmt.Sprintf("markups[%d].categoryId", i), m.CategoryID != uuid.Nil && !seen[m.CategoryID], "invalid")
		seen[m.CategoryID] = true
	}
	return v.Err()
}
