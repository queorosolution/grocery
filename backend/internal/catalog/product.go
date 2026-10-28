package catalog

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/optional"
	"gp/internal/shared/validate"
)

var barcodeRe = regexp.MustCompile(`^[0-9]{6,14}$`)

// Column limits from contracts/schema.sql.
const (
	maxBrand, maxSizeLabel, maxImageURL, maxKeywords = 60, 30, 300, 300
	maxSearch                                        = 60
)

func (s *Service) Products(ctx context.Context, scope db.Scope, q, cur string) (Page[Product], error) {
	after, err := parseCursor(cur)
	if err != nil {
		return Page[Product]{}, err
	}
	if err := validate.New().Str("q", q, 0, maxSearch).Err(); err != nil {
		return Page[Product]{}, err
	}
	var rows []Product
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		rows, err = listProducts(ctx, tx, strings.TrimSpace(q), after)
		return err
	})
	return cut(rows, func(p Product) cursor { return cursor{Name: p.Name, ID: p.ID} }), err
}

func (s *Service) CreateProduct(ctx context.Context, scope db.Scope, in ProductInput) (Product, error) {
	p := Product{Unit: enum.UnitPcs, Status: enum.ProductStatusActive}
	p.apply(in)
	if err := s.checkProduct(p); err != nil {
		return Product{}, err
	}
	var out Product
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		var err error
		if out, err = insertProduct(ctx, tx, p); err != nil {
			return productClash(err)
		}
		return recordProduct(ctx, tx, scope, "product.create", Product{}, out)
	})
	return out, err
}

func (s *Service) UpdateProduct(ctx context.Context, scope db.Scope, id uuid.UUID, in ProductInput) (Product, error) {
	var out Product
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := productByID(ctx, tx, id)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		p := before
		p.apply(in)
		if err := s.checkProduct(p); err != nil {
			return err
		}
		if out, err = saveProduct(ctx, tx, p); err != nil {
			return productClash(err)
		}
		return recordProduct(ctx, tx, scope, "product.update", before, out)
	})
	return out, err
}

// apply writes the fields the body carries onto p and cleans the text.
func (p *Product) apply(in ProductInput) {
	assign(&p.Barcode, in.Barcode)
	assign(&p.Brand, in.Brand)
	assign(&p.SizeLabel, in.SizeLabel)
	assign(&p.WeightG, in.WeightG)
	assign(&p.CategoryID, in.CategoryID)
	assign(&p.ImageURL, in.ImageURL)
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.Unit != nil {
		p.Unit = *in.Unit
	}
	if in.Keywords != nil {
		p.Keywords = strings.TrimSpace(*in.Keywords)
	}
	if in.Status != nil {
		p.Status = *in.Status
	}
	p.Barcode, p.Brand = blankToNil(p.Barcode), blankToNil(p.Brand)
	p.SizeLabel, p.ImageURL = blankToNil(p.SizeLabel), blankToNil(p.ImageURL)
}

func assign[T any](dst **T, f optional.Field[T]) {
	if f.Set {
		*dst = f.Ptr()
	}
}

func (s *Service) checkProduct(p Product) error {
	v := validate.New().
		Str("name", p.Name, 2, 120).
		Str("keywords", p.Keywords, 0, maxKeywords).
		Check("unit", p.Unit.Valid(), "invalid").
		Check("status", p.Status.Valid(), "invalid")
	if p.Barcode != nil {
		v.Check("barcode", barcodeRe.MatchString(*p.Barcode), "invalid")
	}
	if p.Brand != nil {
		v.Str("brand", *p.Brand, 1, maxBrand)
	}
	if p.SizeLabel != nil {
		v.Str("sizeLabel", *p.SizeLabel, 1, maxSizeLabel)
	}
	if p.WeightG != nil {
		v.Int("weightG", int64(*p.WeightG), 1, maxWeightG)
	}
	if p.ImageURL != nil {
		v.HTTPSURLUnder("imageUrl", *p.ImageURL, s.imgBase, maxImageURL)
	}
	return v.Err()
}

func productClash(err error) error {
	switch {
	case db.IsUniqueViolation(err):
		return validate.New().Check("barcode", false, "taken").Err()
	case db.IsForeignKeyViolation(err):
		return validate.New().Check("categoryId", false, "unknown").Err()
	}
	return err
}

func recordProduct(ctx context.Context, tx *db.Tx, scope db.Scope, action string, before, after Product) error {
	e := audit.Entry{Action: action, Entity: "product", EntityID: after.ID.String(), After: after}
	if before.ID != uuid.Nil {
		e.Before = before
	}
	return audit.Write(ctx, tx, scope, e)
}
