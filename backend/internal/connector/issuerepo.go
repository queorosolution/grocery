package connector

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// IssuePayload is what an issue keeps of the source row, so the admin can act
// on it later without the shop's database: no connection data, only the row.
type IssuePayload struct {
	Reason            string     `json:"reason"`
	Name              string     `json:"name"`
	Barcode           *string    `json:"barcode"`
	Brand             *string    `json:"brand"`
	Unit              string     `json:"unit"`
	WeightG           *int       `json:"weightG"`
	ImageURL          *string    `json:"imageUrl"`
	PriceCents        *int64     `json:"priceCents"` // null when the price could not be read
	MRPCents          *int64     `json:"mrpCents"`
	StockQty          *int64     `json:"stockQty"`
	Available         *bool      `json:"available"`
	ShopProductID     *uuid.UUID `json:"shopProductId,omitempty"`     // a held price of a product the shop already sells
	CurrentPriceCents *int64     `json:"currentPriceCents,omitempty"` // what the shop sells it for now
}

func payloadOf(it Item, reason string, m Match) IssuePayload {
	p := IssuePayload{Reason: reason, Name: it.Name, Barcode: it.Barcode, Brand: it.Brand, Unit: it.Unit, WeightG: it.WeightG,
		ImageURL: it.ImageURL, MRPCents: it.MRPCents, StockQty: it.Stock, Available: it.Available}
	if !it.PriceBad {
		p.PriceCents = &it.PriceCents
	}
	if m.Existing != nil {
		id, cur := m.Existing.ShopProductID, m.Existing.ShopPriceCents
		p.ShopProductID, p.CurrentPriceCents = &id, &cur
	}
	return p
}

// raiseIssue opens an issue for a row, or refreshes the open one with the same
// kind and external id. A row the admin chose to ignore stays quiet until its
// content changes.
func raiseIssue(ctx context.Context, tx *db.Tx, c Connector, kind enum.SyncIssueKind, extID string, p IssuePayload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sync_issues (connector_id, shop_id, kind, external_id, payload)
		SELECT $1, $2, $3, $4, $5::jsonb
		WHERE NOT EXISTS (SELECT 1 FROM sync_issues WHERE connector_id = $1 AND kind = $3 AND external_id = $4
		                    AND status = 'IGNORED' AND payload = $5::jsonb)
		ON CONFLICT (connector_id, kind, external_id) WHERE status = 'OPEN' DO UPDATE SET payload = EXCLUDED.payload`,
		c.ID, c.ShopID, kind, extID, string(raw))
	return err
}

// closeIssues marks the open issues of rows that now apply cleanly as resolved
// (by the sync itself, so resolved_by stays empty).
func closeIssues(ctx context.Context, tx *db.Tx, connectorID uuid.UUID, extIDs []string) error {
	if len(extIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE sync_issues SET status = 'RESOLVED', resolved_at = now()
		WHERE connector_id = $1 AND status = 'OPEN' AND external_id = ANY($2)`, connectorID, extIDs)
	return err
}

// Issue is one row of the issue list.
type Issue struct {
	ID          uuid.UUID            `json:"id"`
	ConnectorID uuid.UUID            `json:"connectorId"`
	ShopID      uuid.UUID            `json:"shopId"`
	Kind        enum.SyncIssueKind   `json:"kind"`
	ExternalID  string               `json:"externalId"`
	Payload     IssuePayload         `json:"payload"`
	Status      enum.SyncIssueStatus `json:"status"`
	CreatedAt   time.Time            `json:"createdAt"`
	ResolvedAt  *time.Time           `json:"resolvedAt"`
}

const issueColumns = `id, connector_id, shop_id, kind, external_id, payload, status, created_at, resolved_at`

func scanIssue(row interface{ Scan(...any) error }) (Issue, error) {
	var i Issue
	var raw []byte
	if err := row.Scan(&i.ID, &i.ConnectorID, &i.ShopID, &i.Kind, &i.ExternalID, &raw, &i.Status, &i.CreatedAt, &i.ResolvedAt); err != nil {
		return i, err
	}
	return i, json.Unmarshal(raw, &i.Payload)
}

// issuesBy lists issues newest first, optionally of one status and one shop.
func issuesBy(ctx context.Context, tx *db.Tx, status *enum.SyncIssueStatus, shopID *uuid.UUID, limit int) ([]Issue, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+issueColumns+` FROM sync_issues
		WHERE ($1::text IS NULL OR status = $1) AND ($2::uuid IS NULL OR shop_id = $2)
		ORDER BY created_at DESC, id LIMIT $3`, status, shopID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Issue{}
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func lockIssue(ctx context.Context, tx *db.Tx, id uuid.UUID) (Issue, bool, error) {
	i, err := scanIssue(tx.QueryRow(ctx, `SELECT `+issueColumns+` FROM sync_issues WHERE id = $1 FOR UPDATE`, id))
	return i, err == nil, db.IgnoreNoRows(err)
}

func finishIssue(ctx context.Context, tx *db.Tx, id uuid.UUID, status enum.SyncIssueStatus, by uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE sync_issues SET status = $2, resolved_by = $3, resolved_at = now() WHERE id = $1`, id, status, by)
	return err
}
