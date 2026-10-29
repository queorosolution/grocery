package finance

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ids"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
)

// Payouts is what the platform owes each shop (FR-FIN-03). The amount always
// comes from the ledger, never from the request.
type Payouts struct{ db *db.DB }

func NewPayouts(d *db.DB) *Payouts { return &Payouts{db: d} }

type Payout struct {
	ID          uuid.UUID   `json:"id"`
	ShopID      uuid.UUID   `json:"shopId"`
	PeriodFrom  string      `json:"periodFrom"`
	PeriodTo    string      `json:"periodTo"`
	AmountCents money.Cents `json:"amountCents"`
	Status      string      `json:"status"`
	Reference   *string     `json:"reference"`
	PaidAt      *time.Time  `json:"paidAt"`
	CreatedAt   time.Time   `json:"createdAt"`
}

// maxList is the length of a payout list; there are few payouts per shop.
const maxList = 100

const payoutCols = `id, shop_id, period_from::text, period_to::text, amount_cents, status, reference, paid_at, created_at`

func scanPayout(row interface{ Scan(...any) error }) (p Payout, err error) {
	err = row.Scan(&p.ID, &p.ShopID, &p.PeriodFrom, &p.PeriodTo, &p.AmountCents, &p.Status, &p.Reference, &p.PaidAt, &p.CreatedAt)
	return p, err
}

// Balance is what the platform owes the shop: the sum of the shop's rows. Payout
// rows are negative, so a paid payout lowers it. before limits it to rows written
// before that time; the zero time means all rows.
func Balance(ctx context.Context, tx *db.Tx, shopID uuid.UUID, before time.Time) (money.Cents, error) {
	var sum money.Cents
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_cents), 0)::bigint FROM ledger_entries
		WHERE shop_id = $1 AND party = 'SHOP' AND ($2::timestamptz IS NULL OR created_at < $2)`, shopID, nilTime(before)).Scan(&sum)
	return sum, err
}

func nilTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// ShopView is the owner's page: the balance now and the payouts so far.
type ShopView struct {
	BalanceCents money.Cents `json:"balanceCents"`
	Data         []Payout    `json:"data"`
}

// ForShop is the shop's own view. The shop is the one in the scope.
func (p *Payouts) ForShop(ctx context.Context, scope db.Scope) (ShopView, error) {
	out := ShopView{Data: []Payout{}}
	err := p.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		if out.BalanceCents, err = Balance(ctx, tx, scope.ShopID, time.Time{}); err != nil {
			return err
		}
		out.Data, err = list(ctx, tx, scope.ShopID)
		return err
	})
	return out, err
}

// List is the admin's list, newest first, optionally of one shop.
func (p *Payouts) List(ctx context.Context, scope db.Scope, shopID string) ([]Payout, error) {
	var id uuid.UUID
	if shopID != "" {
		var err error
		if id, err = uuid.Parse(shopID); err != nil {
			return nil, apperr.Validation(map[string]string{"shopId": "invalid"})
		}
	}
	var out []Payout
	err := p.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = list(ctx, tx, id)
		return err
	})
	return out, err
}

func list(ctx context.Context, tx *db.Tx, shopID uuid.UUID) ([]Payout, error) {
	rows, err := tx.Query(ctx, `SELECT `+payoutCols+` FROM payouts WHERE ($1::uuid IS NULL OR shop_id = $1)
		ORDER BY created_at DESC, id LIMIT $2`, ids.Ref(shopID), maxList)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Payout{}
	for rows.Next() {
		p, err := scanPayout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateInput is the body of POST /admin/payouts.
type CreateInput struct {
	ShopID uuid.UUID `json:"shopId"`
	From   string    `json:"from"`
	To     string    `json:"to"`
}

func colomboDate(field, s string) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, s, clock.Colombo(clock.Now()).Location())
	if err != nil {
		return t, apperr.Validation(map[string]string{field: "invalid"})
	}
	return t, nil
}

// Create makes a DRAFT payout. Its amount is the shop's balance at the end of the
// last day of the period, so money from earlier days that no payout covered is
// not lost, and a period can never overlap the one before (nothing is paid twice).
// Only one draft per shop exists at a time, because the balance does not drop
// until a payout is marked paid.
func (p *Payouts) Create(ctx context.Context, scope db.Scope, in CreateInput) (Payout, error) {
	from, errFrom := colomboDate("from", in.From)
	to, errTo := colomboDate("to", in.To)
	v := validate.New().Check("shopId", in.ShopID != uuid.Nil, "required")
	v.Check("from", errFrom == nil, "invalid").Check("to", errTo == nil, "invalid")
	if err := v.Err(); err != nil {
		return Payout{}, err
	}
	today := clock.Colombo(clock.Now())
	v.Check("to", !to.After(today), "future").Check("from", !from.After(to), "after_to")
	if err := v.Err(); err != nil {
		return Payout{}, err
	}
	var out Payout
	err := p.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		// The shop row is the lock that keeps two admins from making two drafts.
		var locked uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM shops WHERE id = $1 FOR UPDATE`, in.ShopID).Scan(&locked); err != nil {
			if db.IgnoreNoRows(err) == nil {
				return apperr.Validation(map[string]string{"shopId": "unknown"})
			}
			return err
		}
		var draft, overlap bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payouts WHERE shop_id = $1 AND status = 'DRAFT'),
			EXISTS (SELECT 1 FROM payouts WHERE shop_id = $1 AND period_to >= $2::date)`, in.ShopID, in.From).Scan(&draft, &overlap); err != nil {
			return err
		}
		switch {
		case draft:
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"payout": "draft_exists"}}
		case overlap:
			return apperr.Validation(map[string]string{"from": "overlaps"})
		}
		amount, err := Balance(ctx, tx, in.ShopID, to.AddDate(0, 0, 1))
		if err != nil {
			return err
		}
		if amount <= 0 {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"amount": "nothing_due"}}
		}
		out, err = scanPayout(tx.QueryRow(ctx, `
			INSERT INTO payouts (shop_id, period_from, period_to, amount_cents, created_by)
			VALUES ($1, $2::date, $3::date, $4, $5) RETURNING `+payoutCols, in.ShopID, in.From, in.To, amount, scope.UserID))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "payout.create", Entity: "payout", EntityID: out.ID.String(), ShopID: in.ShopID,
			After: map[string]any{"from": in.From, "to": in.To, "amountCents": amount}})
	})
	return out, err
}

// PaidInput is the body of POST /admin/payouts/{id}/paid.
type PaidInput struct {
	Reference string `json:"reference"`
}

// MarkPaid records that the money was sent, with the bank's reference. The PAYOUT
// row that takes the amount off the shop's balance is written in the same
// transaction.
func (p *Payouts) MarkPaid(ctx context.Context, scope db.Scope, id uuid.UUID, in PaidInput) (Payout, error) {
	if err := validate.New().Str("reference", in.Reference, 3, 100).Err(); err != nil {
		return Payout{}, err
	}
	var out Payout
	err := p.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		cur, err := scanPayout(tx.QueryRow(ctx, `SELECT `+payoutCols+` FROM payouts WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
		}
		if cur.Status != "DRAFT" {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"status": "not_draft"}}
		}
		out, err = scanPayout(tx.QueryRow(ctx, `UPDATE payouts SET status = 'PAID', reference = $2, paid_at = $3 WHERE id = $1 RETURNING `+payoutCols,
			id, in.Reference, clock.Now()))
		if err != nil {
			return err
		}
		if err := insert(ctx, tx, out.ShopID, uuid.Nil, entry{kind: enum.LedgerTypePayout, party: enum.LedgerPartyShop, amount: -out.AmountCents, payoutID: &id}); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "payout.paid", Entity: "payout", EntityID: id.String(), ShopID: out.ShopID,
			After: map[string]any{"amountCents": out.AmountCents}})
	})
	return out, err
}
