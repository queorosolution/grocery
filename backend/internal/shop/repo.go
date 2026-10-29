package shop

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

const shopColumns = `id, slug, name, phone, address_text, lat, lng, logo_url, theme_color, status, accepting_orders, updated_at`

func scanShop(row pgx.Row) (Shop, error) {
	var s Shop
	err := row.Scan(&s.ID, &s.Slug, &s.Name, &s.Phone, &s.AddressText, &s.Lat, &s.Lng, &s.LogoURL,
		&s.ThemeColor, &s.Status, &s.AcceptingOrders, &s.UpdatedAt)
	return s, err
}

func listShops(ctx context.Context, tx *db.Tx) ([]Shop, error) {
	rows, err := tx.Query(ctx, `SELECT `+shopColumns+` FROM shops ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Shop{}
	for rows.Next() {
		s, err := scanShop(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// shopByID can lock the row, so two admin edits of one shop run one after the other.
func shopByID(ctx context.Context, tx *db.Tx, id uuid.UUID, lock bool) (Shop, bool, error) {
	q := `SELECT ` + shopColumns + ` FROM shops WHERE id = $1`
	if lock {
		q += ` FOR UPDATE`
	}
	s, err := scanShop(tx.QueryRow(ctx, q, id))
	return s, err == nil, db.IgnoreNoRows(err)
}

// insertShop creates the profile and its default settings. New shops start as
// DRAFT and not accepting orders (FR-SHOP-01).
func insertShop(ctx context.Context, tx *db.Tx, p profile) (Shop, error) {
	s, err := scanShop(tx.QueryRow(ctx, `
		INSERT INTO shops (slug, name, phone, address_text, lat, lng, logo_url, theme_color)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+shopColumns,
		p.Slug, p.Name, p.Phone, p.Address, p.Lat, p.Lng, p.Logo, p.ThemeColor))
	if err != nil {
		return Shop{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO shop_settings (shop_id) VALUES ($1)`, s.ID)
	return s, err
}

func saveShop(ctx context.Context, tx *db.Tx, s Shop) (Shop, error) {
	return scanShop(tx.QueryRow(ctx, `
		UPDATE shops SET slug = $2, name = $3, phone = $4, address_text = $5, lat = $6, lng = $7,
		       logo_url = $8, theme_color = $9, status = $10, accepting_orders = $11
		WHERE id = $1 RETURNING `+shopColumns,
		s.ID, s.Slug, s.Name, s.Phone, s.AddressText, s.Lat, s.Lng, s.LogoURL, s.ThemeColor, s.Status, s.AcceptingOrders))
}

func setAccepting(ctx context.Context, tx *db.Tx, id uuid.UUID, on bool) error {
	_, err := tx.Exec(ctx, `UPDATE shops SET accepting_orders = $2 WHERE id = $1`, id, on)
	return err
}

func settingsOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (SettingsView, error) {
	var v SettingsView
	var channels []string
	err := tx.QueryRow(ctx, `
		SELECT delivery_type, payment_provider, payment_account_id, driver_source, notify_channels, markup_bp,
		       min_platform_margin_cents, min_order_cents, max_lines, review_timeout_min, revision_timeout_min,
		       payment_timeout_min, cart_tenant_id, cart_public_key
		FROM shop_settings WHERE shop_id = $1`, id).
		Scan(&v.DeliveryType, &v.PaymentProvider, &v.PaymentAccountID, &v.DriverSource, &channels, &v.MarkupBP,
			&v.MinPlatformMarginCents, &v.MinOrderCents, &v.MaxLines, &v.ReviewTimeoutMin, &v.RevisionTimeoutMin,
			&v.PaymentTimeoutMin, &v.CartTenantID, &v.CartPublicKey)
	for _, c := range channels {
		v.NotifyChannels = append(v.NotifyChannels, enum.NotifyChannel(c))
	}
	return v, err
}

func saveSettings(ctx context.Context, tx *db.Tx, id uuid.UUID, s Settings) error {
	channels := make([]string, len(s.NotifyChannels))
	for i, c := range s.NotifyChannels {
		channels[i] = string(c)
	}
	_, err := tx.Exec(ctx, `
		UPDATE shop_settings SET delivery_type = $2, payment_provider = $3, payment_account_id = $4,
		       driver_source = $5, notify_channels = $6, markup_bp = $7, min_platform_margin_cents = $8,
		       min_order_cents = $9, max_lines = $10, review_timeout_min = $11, revision_timeout_min = $12,
		       payment_timeout_min = $13
		WHERE shop_id = $1`,
		id, s.DeliveryType, s.PaymentProvider, s.PaymentAccountID, s.DriverSource, channels, s.MarkupBP,
		s.MinPlatformMarginCents, s.MinOrderCents, s.MaxLines, s.ReviewTimeoutMin, s.RevisionTimeoutMin, s.PaymentTimeoutMin)
	return err
}

// accountScope reads who owns a payment account, for the settings check.
type accountScope struct {
	Provider enum.PaymentProvider
	Platform bool
	ShopID   *uuid.UUID
}

func accountScopeOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (accountScope, bool, error) {
	var a accountScope
	var owner enum.PaymentAccountOwner
	err := tx.QueryRow(ctx, `SELECT provider, owner_type, shop_id FROM payment_accounts WHERE id = $1`, id).
		Scan(&a.Provider, &owner, &a.ShopID)
	a.Platform = owner == enum.PaymentAccountOwnerPlatform
	return a, err == nil, db.IgnoreNoRows(err)
}

// hoursOf returns the weekly hours as the API shows them.
func hoursOf(ctx context.Context, tx *db.Tx, id uuid.UUID) ([]Day, error) {
	rows, err := tx.Query(ctx, `
		SELECT weekday, (EXTRACT(EPOCH FROM opens) / 60)::int, (EXTRACT(EPOCH FROM closes) / 60)::int
		FROM shop_hours WHERE shop_id = $1 ORDER BY weekday`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Day{}
	for rows.Next() {
		var wd, opens, closes int
		if err := rows.Scan(&wd, &opens, &closes); err != nil {
			return nil, err
		}
		out = append(out, Day{Weekday: wd, Opens: formatClock(opens), Closes: formatClock(closes)})
	}
	return out, rows.Err()
}

func weekOf(days []Day) weekHours {
	w := weekHours{}
	for _, d := range days {
		o, _ := parseClock(d.Opens, false)
		c, _ := parseClock(d.Closes, true)
		w[d.Weekday] = span{opens: o, closes: c}
	}
	return w
}

// replaceHours swaps the whole week in one go. 1440 minutes is "24:00", which
// Postgres keeps as a real time value but cannot get from an interval cast.
func replaceHours(ctx context.Context, tx *db.Tx, id uuid.UUID, week weekHours) error {
	if _, err := tx.Exec(ctx, `DELETE FROM shop_hours WHERE shop_id = $1`, id); err != nil {
		return err
	}
	for wd, sp := range week {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shop_hours (shop_id, weekday, opens, closes)
			VALUES ($1, $2, ($3::int * interval '1 minute')::time,
			        CASE WHEN $4::int = 1440 THEN time '24:00' ELSE ($4::int * interval '1 minute')::time END)`,
			id, wd, sp.opens, sp.closes); err != nil {
			return err
		}
	}
	return nil
}

// snapshotOf loads what the readiness rules need. A usable payment method is an
// ACTIVE account of the provider the settings name, owned by the platform or by
// this shop. MANUAL payments also need the feature switch.
func snapshotOf(ctx context.Context, tx *db.Tx, id uuid.UUID, now time.Time, manualEnabled bool) (snapshot, bool, error) {
	var s snapshot
	var provider enum.PaymentProvider
	err := tx.QueryRow(ctx, `
		SELECT sh.status, sh.accepting_orders, ss.delivery_type, ss.payment_provider,
		       EXISTS (SELECT 1 FROM payment_accounts pa
		               WHERE pa.id = ss.payment_account_id AND pa.status = 'ACTIVE' AND pa.provider = ss.payment_provider
		                 AND (pa.owner_type = 'PLATFORM' OR pa.shop_id = sh.id)),
		       EXISTS (SELECT 1 FROM slots sl
		               WHERE sl.shop_id = sh.id AND sl.status = 'OPEN' AND sl.cutoff_at > $2 AND sl.booked < sl.capacity)
		FROM shops sh JOIN shop_settings ss ON ss.shop_id = sh.id
		WHERE sh.id = $1`, id, now).
		Scan(&s.Status, &s.Accepting, &s.DeliveryType, &provider, &s.PaymentUsable, &s.HasFutureSlot)
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshot{}, false, nil
	}
	if err != nil {
		return snapshot{}, false, err
	}
	if provider == enum.PaymentProviderManual && !manualEnabled {
		s.PaymentUsable = false
	}
	days, err := hoursOf(ctx, tx, id)
	s.Hours = weekOf(days)
	return s, true, err
}

func listPublic(ctx context.Context, tx *db.Tx) ([]PublicShop, error) {
	rows, err := tx.Query(ctx, `SELECT slug, name, logo_url, updated_at FROM shops WHERE status = 'ACTIVE' ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PublicShop{}
	for rows.Next() {
		var p PublicShop
		if err := rows.Scan(&p.Slug, &p.Name, &p.LogoURL, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func existsShop(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM shops WHERE id = $1)`, id).Scan(&ok)
	return ok, err
}

func activeIDBySlug(ctx context.Context, tx *db.Tx, slug string) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM shops WHERE slug = $1 AND status = 'ACTIVE'`, slug).Scan(&id)
	return id, err == nil, db.IgnoreNoRows(err)
}

func activeLocations(ctx context.Context, tx *db.Tx) ([]Location, error) {
	rows, err := tx.Query(ctx, `SELECT id, slug, name, lat, lng FROM shops WHERE status = 'ACTIVE' ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.Slug, &l.Name, &l.Lat, &l.Lng); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
