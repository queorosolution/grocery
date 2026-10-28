package shop

import (
	"context"
	"time"

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
}

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
			return notFoundOr(err, found)
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

// SetHours replaces the weekly hours. An empty list means closed every day.
func (s *Service) SetHours(ctx context.Context, scope db.Scope, id uuid.UUID, days []Day) (Detail, error) {
	week, err := parseHours(days)
	if err != nil {
		return Detail{}, err
	}
	var d Detail
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if _, found, err := shopByID(ctx, tx, id, true); err != nil || !found {
			return notFoundOr(err, found)
		}
		before, err := hoursOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := replaceHours(ctx, tx, id, week); err != nil {
			return err
		}
		after, err := hoursOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{
			Action: "shop.hours.update", Entity: "shop_hours", EntityID: id.String(), ShopID: id,
			Before: before, After: after,
		}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

// SetAccepting switches order taking on or off. Switching on is refused with
// SHOP_NOT_READY and the list of what is missing; off always works.
func (s *Service) SetAccepting(ctx context.Context, scope db.Scope, id uuid.UUID, on bool) (Detail, error) {
	var d Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := shopByID(ctx, tx, id, true)
		if err != nil || !found {
			return notFoundOr(err, found)
		}
		if on {
			r, err := s.readiness(ctx, tx, id)
			if err != nil {
				return err
			}
			if !r.Ready {
				fields := map[string]string{}
				for _, m := range r.Missing {
					fields[string(m)] = "missing"
				}
				return &apperr.Error{Code: apperr.ShopNotReady, Fields: fields}
			}
		}
		if err := setAccepting(ctx, tx, id, on); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{
			Action: "shop.accepting", Entity: "shop", EntityID: id.String(), ShopID: id,
			Before: map[string]bool{"acceptingOrders": before.AcceptingOrders}, After: map[string]bool{"acceptingOrders": on},
		}); err != nil {
			return err
		}
		d, err = s.detail(ctx, tx, id)
		return err
	})
	return d, err
}

// CanTakeOrders is for the order module, inside its transaction. It returns nil
// when a bill may be submitted now, SHOP_CLOSED when the shop is switched off
// or outside its hours, and SHOP_NOT_READY when its setup is incomplete.
func (s *Service) CanTakeOrders(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time) error {
	snap, found, err := snapshotOf(ctx, tx, id, now, s.manualEnabled)
	if err != nil {
		return err
	}
	if !found {
		return apperr.New(apperr.NotFound)
	}
	switch snap.canTakeOrders(now) {
	case Closed:
		return apperr.New(apperr.ShopClosed)
	case NotReady:
		return apperr.New(apperr.ShopNotReady)
	}
	return nil
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

func notFoundOr(err error, found bool) error {
	if err != nil {
		return err
	}
	if !found {
		return apperr.New(apperr.NotFound)
	}
	return nil
}

func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Exists lets other modules (delivery) check a shop id inside their own transaction.
func (s *Service) Exists(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error) {
	return existsShop(ctx, tx, id)
}

// IDBySlug resolves the slug of an ACTIVE shop; suspended and draft shops are not found.
func (s *Service) IDBySlug(ctx context.Context, tx *db.Tx, slug string) (uuid.UUID, bool, error) {
	return activeIDBySlug(ctx, tx, slug)
}

// ActiveLocations is where each active shop is, for "which shops deliver to this pin".
func (s *Service) ActiveLocations(ctx context.Context, tx *db.Tx) ([]Location, error) {
	return activeLocations(ctx, tx)
}
