package connector

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
)

// Actions on a sync issue (docs/06 section 10).
const (
	ResolveLink         = "LINK"          // UNMATCHED: sell an existing master product under this id
	ResolveCreate       = "CREATE"        // UNMATCHED: make a new master product from the row
	ResolveApprovePrice = "APPROVE_PRICE" // PRICE_HOLD: take the held price after all
	ResolveRejectPrice  = "REJECT_PRICE"  // PRICE_HOLD: keep the old price
	ResolveIgnore       = "IGNORE"        // either: leave the row alone until it changes
)

const issuesShown = 100

// ResolveInput is the body of POST /sync-issues/{id}/resolve.
type ResolveInput struct {
	Action    string     `json:"action"`
	ProductID *uuid.UUID `json:"productId"` // LINK only
}

// Issues lists issues, newest first. status and shopID are optional filters.
func (s *Service) Issues(ctx context.Context, scope db.Scope, status *enum.SyncIssueStatus, shopID *uuid.UUID) ([]Issue, error) {
	var out []Issue
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = issuesBy(ctx, tx, status, shopID, issuesShown)
		return err
	})
	return out, err
}

// Resolve acts on one open issue. The action and the change it causes happen in
// one transaction, so an issue is never closed without its effect. An issue that
// is not open is a CONFLICT, which makes a double click harmless.
func (s *Service) Resolve(ctx context.Context, scope db.Scope, id uuid.UUID, in ResolveInput) (Issue, error) {
	var out Issue
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		issue, found, err := lockIssue(ctx, tx, id)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if issue.Status != enum.SyncIssueStatusOpen {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"status": "not_open"}}
		}
		c, found, err := byID(ctx, tx, issue.ConnectorID, false)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		status, err := s.act(ctx, tx, scope, c, issue, in)
		if err != nil {
			return err
		}
		if err := finishIssue(ctx, tx, id, status, scope.UserID); err != nil {
			return err
		}
		out = issue
		out.Status = status
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "sync_issue.resolve", Entity: "sync_issue", EntityID: id.String(), ShopID: issue.ShopID,
			After: map[string]any{"action": in.Action, "kind": issue.Kind, "externalId": issue.ExternalID},
		})
	})
	return out, err
}

func notAllowed() error { return validate.New().Check("action", false, "not_allowed").Err() }

// act does the work of an action and returns the status the issue ends in.
func (s *Service) act(ctx context.Context, tx *db.Tx, scope db.Scope, c Connector, issue Issue, in ResolveInput) (enum.SyncIssueStatus, error) {
	it := itemOf(issue)
	switch {
	case in.Action == ResolveIgnore:
		return enum.SyncIssueStatusIgnored, nil
	case in.Action == ResolveRejectPrice && issue.Kind == enum.SyncIssueKindPriceHold:
		return enum.SyncIssueStatusResolved, nil
	case in.Action == ResolveLink && issue.Kind == enum.SyncIssueKindUnmatched:
		if in.ProductID == nil {
			return "", validate.New().Check("productId", false, "required").Err()
		}
		return enum.SyncIssueStatusResolved, s.sellNew(ctx, tx, scope, c, it, *in.ProductID, false)
	case in.Action == ResolveCreate && issue.Kind == enum.SyncIssueKindUnmatched:
		return enum.SyncIssueStatusResolved, s.sellNew(ctx, tx, scope, c, it, uuid.Nil, false)
	case in.Action == ResolveApprovePrice && issue.Kind == enum.SyncIssueKindPriceHold:
		// A price of 0 or one that cannot be read is never approved: reject it or ignore it.
		if issue.Payload.Reason != ReasonOverMRP && issue.Payload.Reason != ReasonBigDrop {
			return "", notAllowed()
		}
		return enum.SyncIssueStatusResolved, s.approve(ctx, tx, scope, c, issue, it)
	}
	return "", notAllowed()
}

// sellNew puts the row on sale as a new shop product. With no product id it
// makes the master product from the row first. dropMRP leaves out an MRP that
// is below the price being approved.
func (s *Service) sellNew(ctx context.Context, tx *db.Tx, scope db.Scope, c Connector, it Item, productID uuid.UUID, dropMRP bool) error {
	if reason := priceHold(it, nil); reason != "" && !dropMRP {
		return validate.New().Check("price", false, reason).Err()
	}
	if productID == uuid.Nil {
		p, err := s.rt.Products.CreateProductIn(ctx, tx, scope, masterInput(it.Name, it.Barcode, it.Brand, it.ImageURL, it.Unit, it.WeightG))
		if err != nil {
			return err
		}
		productID = p.ID
	}
	var mrp *money.Cents
	if !dropMRP {
		mrp = mrpOf(it, nil)
	}
	_, err := s.addProduct(ctx, tx, scope, c.ShopID, it, productID, mrp)
	if db.IsUniqueViolation(err) {
		return validate.New().Check("productId", false, "taken").Err()
	}
	return err
}

// approve takes a held price. If the price is above the MRP, the MRP is dropped
// (the source's MRP is then known to be out of date); a drop of more than half
// is taken as it is.
func (s *Service) approve(ctx context.Context, tx *db.Tx, scope db.Scope, c Connector, issue Issue, it Item) error {
	price := money.Cents(it.PriceCents)
	mrp := mrpOf(it, nil)
	if p := issue.Payload.ShopProductID; p != nil {
		if mrp == nil {
			var cur *int64
			if err := tx.QueryRow(ctx, `SELECT mrp_cents FROM shop_products WHERE id = $1`, *p).Scan(&cur); err != nil {
				return apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
			}
			if cur != nil {
				v := money.Cents(*cur)
				mrp = &v
			}
		}
		if mrp != nil && price > *mrp {
			mrp = nil
		}
		_, err := s.rt.Pricer.SetShopPrice(ctx, tx, scope, c.ShopID, *p, price, mrp, enum.PriceChangeSourceConnector)
		return err
	}
	// The held row was a new product: find it by barcode, or make it.
	var productID uuid.UUID
	if it.Barcode != nil {
		err := tx.QueryRow(ctx, `SELECT id FROM products WHERE barcode = $1`, *it.Barcode).Scan(&productID)
		if err = db.IgnoreNoRows(err); err != nil {
			return err
		}
	}
	return s.sellNew(ctx, tx, scope, c, it, productID, true)
}

// itemOf rebuilds the cleaned row from what an issue kept.
func itemOf(i Issue) Item {
	p := i.Payload
	it := Item{ExternalID: i.ExternalID, Name: p.Name, Barcode: p.Barcode, Brand: p.Brand, MRPCents: p.MRPCents,
		Stock: p.StockQty, Available: p.Available, Unit: p.Unit, WeightG: p.WeightG, ImageURL: p.ImageURL}
	if p.PriceCents == nil {
		it.PriceBad = true
	} else {
		it.PriceCents = *p.PriceCents
	}
	if it.Unit == "" {
		it.Unit = "PCS"
	}
	return it
}
