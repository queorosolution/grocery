package shop

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/events"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/secrets"
	"gp/internal/shared/validate"
)

type Service struct {
	db            *db.DB
	secrets       secrets.Putter
	manualEnabled bool // FEATURE_MANUAL_PAYMENT
	repricer      Repricer
}

// Repricer recomputes a shop's customer prices. The pricing module implements
// it; it is set after both services exist because pricing reads the markup
// from this service.
type Repricer interface {
	Reprice(ctx context.Context, tx *db.Tx, scope db.Scope, shopID uuid.UUID) error
}

// SetRepricer wires the pricing module. Without one, a markup change does not
// touch prices (tests of other modules).
func (s *Service) SetRepricer(r Repricer) { s.repricer = r }

func NewService(d *db.DB, s secrets.Putter, manualPayments bool) *Service {
	return &Service{db: d, secrets: s, manualEnabled: manualPayments}
}

// PublicList is the active shops for the site build. It reads without a user.
func (s *Service) PublicList(ctx context.Context) ([]PublicShop, error) {
	var out []PublicShop
	err := s.db.Tx(ctx, db.Public(), func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = listPublic(ctx, tx)
		return err
	})
	return out, err
}

func (s *Service) List(ctx context.Context, scope db.Scope) ([]Shop, error) {
	var out []Shop
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = listShops(ctx, tx)
		return err
	})
	return out, err
}

func (s *Service) Get(ctx context.Context, scope db.Scope, id uuid.UUID) (Detail, error) {
	var d Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

func (s *Service) detail(ctx context.Context, tx *db.Tx, id uuid.UUID) (Detail, error) {
	sh, found, err := shopByID(ctx, tx, id, false)
	if err != nil {
		return Detail{}, err
	}
	if !found {
		return Detail{}, apperr.New(apperr.NotFound)
	}
	d := Detail{Shop: sh}
	if d.Settings, err = settingsOf(ctx, tx, id); err != nil {
		return Detail{}, err
	}
	if d.Hours, err = hoursOf(ctx, tx, id); err != nil {
		return Detail{}, err
	}
	d.Readiness, err = s.readiness(ctx, tx, id)
	return d, err
}

func (s *Service) readiness(ctx context.Context, tx *db.Tx, id uuid.UUID) (Readiness, error) {
	snap, found, err := snapshotOf(ctx, tx, id, clock.Now(), s.manualEnabled)
	if err != nil {
		return Readiness{}, err
	}
	if !found {
		return Readiness{}, apperr.New(apperr.NotFound)
	}
	return snap.setup(), nil
}

func (s *Service) Readiness(ctx context.Context, scope db.Scope, id uuid.UUID) (Readiness, error) {
	var r Readiness
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		r, err = s.readiness(ctx, tx, id)
		return err
	})
	return r, err
}

// Create adds a DRAFT shop that is not accepting orders (FR-SHOP-01).
func (s *Service) Create(ctx context.Context, scope db.Scope, in CreateInput) (Detail, error) {
	p := profileFromCreate(in)
	p.clean()
	if err := p.validate(); err != nil {
		return Detail{}, err
	}
	var d Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		created, err := insertShop(ctx, tx, p)
		if err != nil {
			return slugTaken(err)
		}
		if err := s.record(ctx, tx, scope, "shop.create", Shop{}, created, true); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, created.ID)
		return err
	})
	return d, err
}

// Update changes the profile. A new slug, name, logo or status asks for a site
// rebuild, because those are baked into the pre-built shop pages.
func (s *Service) Update(ctx context.Context, scope db.Scope, id uuid.UUID, in UpdateInput) (Detail, error) {
	var d Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := shopByID(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.NotFound)
		}
		p := profileOf(before).apply(in)
		p.clean()
		after := before
		after.Slug, after.Name, after.Phone, after.AddressText = p.Slug, p.Name, p.Phone, p.Address
		after.Lat, after.Lng, after.LogoURL, after.ThemeColor = p.Lat, p.Lng, p.Logo, p.ThemeColor
		if in.Status != nil {
			if err := validate.New().Check("status", in.Status.Valid(), "invalid").Err(); err != nil {
				return err
			}
			after.Status = *in.Status
		}
		if err := p.validate(); err != nil {
			return err
		}
		if after.Status != enum.ShopStatusActive {
			after.AcceptingOrders = false // a shop that is not live cannot take orders
		}
		if after, err = saveShop(ctx, tx, after); err != nil {
			return slugTaken(err)
		}
		rebuild := before.Slug != after.Slug || before.Name != after.Name ||
			!sameText(before.LogoURL, after.LogoURL) || before.Status != after.Status
		if err := s.record(ctx, tx, scope, "shop.update", before, after, rebuild); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

// UpdateSettings replaces all per-shop options.
func (s *Service) UpdateSettings(ctx context.Context, scope db.Scope, id uuid.UUID, in Settings) (Detail, error) {
	if err := validateSettings(in); err != nil {
		return Detail{}, err
	}
	var d Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if _, found, err := shopByID(ctx, tx, id, true); err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if err := s.checkAccount(ctx, tx, id, in); err != nil {
			return err
		}
		before, err := settingsOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := saveSettings(ctx, tx, id, in); err != nil {
			return err
		}
		if s.repricer != nil && before.MarkupBP != in.MarkupBP {
			if err := s.repricer.Reprice(ctx, tx, scope, id); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{
			Action: "shop.settings.update", Entity: "shop_settings", EntityID: id.String(), ShopID: id,
			Before: before.Settings, After: in,
		}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

// checkAccount makes sure the chosen payment account belongs to this shop (or
// the platform) and speaks the chosen provider.
func (s *Service) checkAccount(ctx context.Context, tx *db.Tx, shopID uuid.UUID, in Settings) error {
	if in.PaymentAccountID == nil {
		return nil
	}
	a, found, err := accountScopeOf(ctx, tx, *in.PaymentAccountID)
	if err != nil {
		return err
	}
	v := validate.New().Check("paymentAccountId", found && (a.Platform || (a.ShopID != nil && *a.ShopID == shopID)), "unknown")
	if found {
		v.Check("paymentProvider", a.Provider == in.PaymentProvider, "provider_mismatch")
	}
	return v.Err()
}

// record writes the audit row and, when asked, the site.rebuild event.
func (s *Service) record(ctx context.Context, tx *db.Tx, scope db.Scope, action string, before, after Shop, rebuild bool) error {
	e := audit.Entry{Action: action, Entity: "shop", EntityID: after.ID.String(), ShopID: after.ID, After: after}
	if before.ID != uuid.Nil {
		e.Before = before
	}
	if err := audit.Write(ctx, tx, scope, e); err != nil {
		return err
	}
	if !rebuild {
		return nil
	}
	_, err := events.Add(ctx, tx, events.Event{
		Type: enum.EventTypeSiteRebuild, ShopID: after.ID, EntityID: after.ID.String(),
		Payload: map[string]string{"reason": action},
	})
	return err
}

func slugTaken(err error) error {
	if db.IsUniqueViolation(err) {
		return validate.New().Check("slug", false, "taken").Err()
	}
	return err
}

func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
