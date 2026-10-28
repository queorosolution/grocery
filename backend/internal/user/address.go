package user

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shared/validate"
)

const (
	maxAddresses = 5
	maxLabel     = 30
	minLine1     = 3
	maxLine1     = 160
	maxLandmark  = 120
)

// clean trims the text fields; the stored value is what was validated.
func (a *Address) clean() {
	a.Label, a.Line1 = strings.TrimSpace(a.Label), strings.TrimSpace(a.Line1)
	if a.Landmark != nil {
		l := strings.TrimSpace(*a.Landmark)
		a.Landmark = &l
	}
}

func (a Address) validate() error {
	v := validate.New().
		Str("label", a.Label, 1, maxLabel).
		Str("line1", a.Line1, minLine1, maxLine1).
		Check("lat", a.Lat >= geo.MinLat && a.Lat <= geo.MaxLat, "out_of_range").
		Check("lng", a.Lng >= geo.MinLng && a.Lng <= geo.MaxLng, "out_of_range")
	if a.Landmark != nil {
		v.Check("landmark", utf8.RuneCountInString(*a.Landmark) <= maxLandmark, "too_long")
	}
	return v.Err()
}

// Addresses lists the caller's addresses, the default first.
func (s *Service) Addresses(ctx context.Context, scope db.Scope) ([]Address, error) {
	var out []Address
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = listAddresses(ctx, tx, scope.UserID)
		return err
	})
	return out, err
}

// AddAddress saves an address. The first address is the default; so is one sent
// with isDefault. The answer lists the shops that deliver to the pin.
func (s *Service) AddAddress(ctx context.Context, scope db.Scope, in AddressInput) (AddressResult, error) {
	a := Address{Label: in.Label, Line1: in.Line1, Landmark: in.Landmark, Lat: in.Lat, Lng: in.Lng}
	a.clean()
	if err := a.validate(); err != nil {
		return AddressResult{}, err
	}
	in.Label, in.Line1, in.Landmark = a.Label, a.Line1, a.Landmark
	var out AddressResult
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := lockUser(ctx, tx, scope.UserID); err != nil {
			return err
		}
		n, err := countAddresses(ctx, tx, scope.UserID)
		if err != nil {
			return err
		}
		if n >= maxAddresses {
			return apperr.Validation(map[string]string{"addresses": "max_5"})
		}
		isDefault := in.IsDefault || n == 0
		if isDefault {
			if err := clearDefault(ctx, tx, scope.UserID); err != nil {
				return err
			}
		}
		if out.Address, err = insertAddress(ctx, tx, scope.UserID, in, isDefault); err != nil {
			return err
		}
		out.DeliveringShops, err = s.reach.DeliveringShops(ctx, tx, geo.Point{Lat: in.Lat, Lng: in.Lng})
		return err
	})
	return out, err
}

// UpdateAddress changes some fields of one address. Making it the default moves
// the default; the default cannot be switched off, only moved to another address.
func (s *Service) UpdateAddress(ctx context.Context, scope db.Scope, id uuid.UUID, p AddressPatch) (AddressResult, error) {
	if p.Label == nil && p.Line1 == nil && p.Landmark == nil && p.Lat == nil && p.Lng == nil && p.IsDefault == nil {
		return AddressResult{}, apperr.Validation(map[string]string{"body": "empty"})
	}
	var out AddressResult
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := lockUser(ctx, tx, scope.UserID); err != nil {
			return err
		}
		a, found, err := addressByID(ctx, tx, scope.UserID, id)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.NotFound)
		}
		if err := apply(&a, p); err != nil {
			return err
		}
		if p.IsDefault != nil && *p.IsDefault {
			if err := clearDefault(ctx, tx, scope.UserID); err != nil {
				return err
			}
		}
		if out.Address, err = saveAddress(ctx, tx, scope.UserID, a); err != nil {
			return err
		}
		out.DeliveringShops, err = s.reach.DeliveringShops(ctx, tx, geo.Point{Lat: a.Lat, Lng: a.Lng})
		return err
	})
	return out, err
}

func apply(a *Address, p AddressPatch) error {
	if p.Label != nil {
		a.Label = *p.Label
	}
	if p.Line1 != nil {
		a.Line1 = *p.Line1
	}
	if p.Landmark != nil {
		a.Landmark = p.Landmark
	}
	if p.Lat != nil {
		a.Lat = *p.Lat
	}
	if p.Lng != nil {
		a.Lng = *p.Lng
	}
	if p.IsDefault != nil {
		if !*p.IsDefault && a.IsDefault {
			return apperr.Validation(map[string]string{"isDefault": "choose_another_default"})
		}
		a.IsDefault = *p.IsDefault
	}
	a.clean()
	return a.validate()
}

// DeleteAddress removes an address. If it was the default, the newest remaining one becomes the default.
func (s *Service) DeleteAddress(ctx context.Context, scope db.Scope, id uuid.UUID) error {
	return s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := lockUser(ctx, tx, scope.UserID); err != nil {
			return err
		}
		if _, found, err := addressByID(ctx, tx, scope.UserID, id); err != nil || !found {
			if err == nil {
				err = apperr.New(apperr.NotFound)
			}
			return err
		}
		if err := deleteAddress(ctx, tx, scope.UserID, id); err != nil {
			return err
		}
		return promoteNewest(ctx, tx, scope.UserID)
	})
}
