package delivery

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
	"gp/internal/shop"
)

// Shops is what delivery needs from the shop module. Delivery does not read
// the shops table itself.
type Shops interface {
	ActiveLocations(ctx context.Context, tx *db.Tx) ([]shop.Location, error)
	Exists(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error)
	IDBySlug(ctx context.Context, tx *db.Tx, slug string) (uuid.UUID, bool, error)
}

type Service struct {
	db    *db.DB
	shops Shops
}

func NewService(d *db.DB, shops Shops) *Service { return &Service{db: d, shops: shops} }

func (s *Service) requireShop(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	ok, err := s.shops.Exists(ctx, tx, id)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.New(apperr.NotFound)
	}
	return nil
}

// Rules lists the platform default first, then the shop rules.
func (s *Service) Rules(ctx context.Context, scope db.Scope) ([]RuleRow, error) {
	var out []RuleRow
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = listRules(ctx, tx)
		return err
	})
	return out, err
}

// SetRule replaces the rule of a shop, or the platform default when shopID is nil.
func (s *Service) SetRule(ctx context.Context, scope db.Scope, shopID *uuid.UUID, in Rule) (RuleRow, error) {
	if err := in.Validate(); err != nil {
		return RuleRow{}, err
	}
	var out RuleRow
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		entity := uuid.Nil
		if shopID != nil {
			if err := s.requireShop(ctx, tx, *shopID); err != nil {
				return err
			}
			entity = *shopID
		}
		before, had, err := exactRule(ctx, tx, shopID)
		if err != nil {
			return err
		}
		if out, err = saveRule(ctx, tx, shopID, in); err != nil {
			return err
		}
		e := audit.Entry{Action: "fee_rule.set", Entity: "fee_rule", ShopID: entity, After: out.Rule}
		if shopID == nil {
			e.EntityID = "default"
		} else {
			e.EntityID = entity.String()
		}
		if had {
			e.Before = before.Rule
		}
		return audit.Write(ctx, tx, scope, e)
	})
	return out, err
}

// RuleFor is the rule that prices a shop's deliveries: its own, else the
// platform default. With neither, the shop cannot price a delivery yet.
// The bill builder and the address check use it.
func (s *Service) RuleFor(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (RuleRow, error) {
	r, found, err := ruleFor(ctx, tx, shopID)
	if err != nil {
		return RuleRow{}, err
	}
	if !found {
		return RuleRow{}, apperr.New(apperr.ShopNotReady)
	}
	return r, nil
}

// PreviewInput is an admin "what would this cost" question. Distance is the
// straight line in km; the road factor is applied by Fee.
type PreviewInput struct {
	ShopID     uuid.UUID   `json:"shopId"`
	DistanceKm Dec2        `json:"distanceKm"`
	WeightKg   Dec2        `json:"weightKg"`
	ItemsCents money.Cents `json:"itemsCents"`
}

type PreviewResult struct {
	FeeCents int64 `json:"feeCents"`
	// Rule says which rule priced it: the shop's own or the platform default.
	Rule string `json:"rule"`
}

func (s *Service) Preview(ctx context.Context, scope db.Scope, in PreviewInput) (PreviewResult, error) {
	if err := validate.New().Int("itemsCents", int64(in.ItemsCents), 0, maxMoney).Err(); err != nil {
		return PreviewResult{}, err
	}
	var out PreviewResult
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := s.requireShop(ctx, tx, in.ShopID); err != nil {
			return err
		}
		row, err := s.RuleFor(ctx, tx, in.ShopID)
		if err != nil {
			return err
		}
		fee, err := Fee(row.Rule, int(in.DistanceKm)*10, int(in.WeightKg)*10, in.ItemsCents)
		if err != nil {
			return err
		}
		out = PreviewResult{FeeCents: int64(fee), Rule: "DEFAULT"}
		if row.ShopID != nil {
			out.Rule = "SHOP"
		}
		return nil
	})
	return out, err
}
