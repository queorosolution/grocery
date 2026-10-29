package shop

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/geo"
)

// What other modules ask of a shop, inside their own transaction.

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

// MarkupBP is the shop's default markup, for the pricing module inside its own
// transaction. found is false for an unknown shop.
func (s *Service) MarkupBP(ctx context.Context, tx *db.Tx, id uuid.UUID) (bp int, found bool, err error) {
	return markupOf(ctx, tx, id)
}

// BillFacts is what the bill builder reads about a shop's rules. It does not
// look at payment readiness, so any scope may read it: a revision of an order
// that is already in flight needs the numbers but not the verdict.
type BillFacts struct {
	Location               geo.Point
	MinOrderCents          int64
	MinPlatformMarginCents int64
	MaxLines               int
}

// OrderFacts is what the bill builder reads about a shop that takes new orders.
type OrderFacts struct {
	BillFacts
	Slug string // the order code prefix comes from it
	// CartTenant is empty while the shop's cart is not enabled.
	CartTenant string
	// DeliveryType and PaymentProvider are copied onto an order when it is submitted.
	DeliveryType    enum.DeliveryType
	PaymentProvider enum.PaymentProvider
	// Verdict is CanOrder (empty) when the shop is accepting orders now, else why not.
	Verdict Verdict
}

// Open is true when CanTakeOrders would pass.
func (f OrderFacts) Open() bool { return f.Verdict == CanOrder }

// BillFacts reads the shop's pricing rules, whatever its status.
func (s *Service) BillFacts(ctx context.Context, tx *db.Tx, id uuid.UUID) (BillFacts, error) {
	var loc geo.Point
	err := tx.QueryRow(ctx, `SELECT lat, lng FROM shops WHERE id = $1`, id).Scan(&loc.Lat, &loc.Lng)
	if err != nil {
		return BillFacts{}, apperr.NotFoundUnless(db.IgnoreNoRows(err), false)
	}
	set, err := settingsOf(ctx, tx, id)
	if err != nil {
		return BillFacts{}, err
	}
	return billFactsOf(loc, set), nil
}

func billFactsOf(loc geo.Point, set SettingsView) BillFacts {
	return BillFacts{Location: loc, MinOrderCents: set.MinOrderCents, MinPlatformMarginCents: set.MinPlatformMarginCents, MaxLines: set.MaxLines}
}

// OrderFacts gathers them inside the caller's transaction. A shop that is not
// ACTIVE is not found, so a customer cannot tell a draft shop from a missing one.
func (s *Service) OrderFacts(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time) (OrderFacts, error) {
	var f OrderFacts
	var loc geo.Point
	err := tx.QueryRow(ctx, `SELECT slug, lat, lng FROM shops WHERE id = $1 AND status = 'ACTIVE'`, id).Scan(&f.Slug, &loc.Lat, &loc.Lng)
	if err != nil {
		return f, apperr.NotFoundUnless(db.IgnoreNoRows(err), false)
	}
	set, err := settingsOf(ctx, tx, id)
	if err != nil {
		return f, err
	}
	f.BillFacts = billFactsOf(loc, set)
	f.DeliveryType, f.PaymentProvider = set.DeliveryType, set.PaymentProvider
	if set.CartTenantID != nil {
		f.CartTenant = *set.CartTenantID
	}
	snap, found, err := snapshotOf(ctx, tx, id, now, s.manualEnabled)
	if err != nil || !found {
		return f, apperr.NotFoundUnless(err, found)
	}
	f.Verdict = snap.canTakeOrders(now)
	return f, nil
}

// Timeouts are how long each wait of an order lasts, from the shop's settings.
type Timeouts struct {
	Review, Revision, Payment time.Duration
}

// TimeoutsOf reads them for the order module's timers and expiry times.
func (s *Service) TimeoutsOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (Timeouts, error) {
	set, err := settingsOf(ctx, tx, id)
	if err != nil {
		return Timeouts{}, err
	}
	minutes := func(m int) time.Duration { return time.Duration(m) * time.Minute }
	return Timeouts{Review: minutes(set.ReviewTimeoutMin), Revision: minutes(set.RevisionTimeoutMin), Payment: minutes(set.PaymentTimeoutMin)}, nil
}

// DriverSourceOf is who may deliver this shop's orders (FR-DLV-02). A shop that
// does not exist is NOT_FOUND.
func (s *Service) DriverSourceOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (enum.DriverSource, error) {
	var src enum.DriverSource
	err := tx.QueryRow(ctx, `SELECT driver_source FROM shop_settings WHERE shop_id = $1`, id).Scan(&src)
	return src, apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
}

// NotifyTarget is who the notify consumers write to for a shop and how: the
// shop's name for texts, the phone of its desk, and the channels it chose
// (none chosen means it wants no messages).
type NotifyTarget struct {
	Name     string
	Phone    string // empty when the shop has no phone
	Channels []enum.NotifyChannel
}

// NotifyTarget reads it inside the caller's transaction. An unknown shop is NOT_FOUND.
func (s *Service) NotifyTarget(ctx context.Context, tx *db.Tx, id uuid.UUID) (NotifyTarget, error) {
	var t NotifyTarget
	var phone *string
	var channels []string
	err := tx.QueryRow(ctx, `SELECT sh.name, sh.phone, ss.notify_channels FROM shops sh JOIN shop_settings ss ON ss.shop_id = sh.id WHERE sh.id = $1`, id).
		Scan(&t.Name, &phone, &channels)
	if err != nil {
		return t, apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
	}
	if phone != nil {
		t.Phone = *phone
	}
	for _, c := range channels {
		t.Channels = append(t.Channels, enum.NotifyChannel(c))
	}
	return t, nil
}
