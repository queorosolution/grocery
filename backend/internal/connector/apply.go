package connector

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"

	"gp/internal/catalog"
	"gp/internal/connector/source"
	"gp/internal/events"
	"gp/internal/pricing"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/optional"
)

// defaultMaxPerOrder is the limit of a shop product the sync creates; the shop
// changes it by hand later (it is the one field a connector never touches).
const defaultMaxPerOrder = 20

// batch is the state of one transaction of up to 200 rows. Two rows of the same
// batch can point at the same master product, which loadMatches cannot see yet.
type batch struct {
	s       *Service
	tx      *db.Tx
	c       Connector
	counts  Counts
	applied []string             // external ids that applied cleanly: their open issues are closed
	linked  map[uuid.UUID]bool   // master products linked to the shop by this batch
	made    map[string]uuid.UUID // barcode -> master product created by this batch
}

type sourceRow struct {
	item Item
	hash []byte
}

// applyBatch saves a slice of the rows in the caller's transaction. It returns
// what it did and the external ids it saw (for the missing-rows step).
func (s *Service) applyBatch(ctx context.Context, tx *db.Tx, c Connector, rows []source.Row, slot map[string]int, full bool) (Counts, []string, error) {
	b := &batch{s: s, tx: tx, c: c, linked: map[uuid.UUID]bool{}, made: map[string]uuid.UUID{}}
	// A source can hold the same id twice; the last row wins, so one id is applied once.
	byID := map[string]int{}
	var list []sourceRow
	for _, row := range rows {
		b.counts.Read++
		it, bad := Normalise(fieldsOf(row, slot), c.MoneyUnit, s.imgBase)
		if bad != "" {
			continue
		}
		sr := sourceRow{item: it, hash: hashRow(rowValues(row, c))}
		if i, dup := byID[it.ExternalID]; dup {
			list[i] = sr
			continue
		}
		byID[it.ExternalID] = len(list)
		list = append(list, sr)
	}
	items := make([]Item, len(list))
	seen := make([]string, len(list))
	for i, r := range list {
		items[i], seen[i] = r.item, r.item.ExternalID
	}
	matches, err := loadMatches(ctx, tx, c.ShopID, items)
	if err != nil {
		return Counts{}, nil, err
	}
	for _, r := range list {
		m := b.share(r.item, matches[r.item.ExternalID])
		if full && m.Existing != nil && bytes.Equal(m.Existing.SourceHash, r.hash) {
			continue // the row is the same as when it was last applied
		}
		if err := b.row(ctx, r, m); err != nil {
			return Counts{}, nil, err
		}
	}
	return b.counts, seen, closeIssues(ctx, tx, c.ID, b.applied)
}

// rowValues is the part of a read row that holds the mapped columns (without
// the change time that an incremental read adds at the end).
func rowValues(row source.Row, c Connector) source.Row {
	if c.changeColumn() != "" {
		return row[:len(row)-1]
	}
	return row
}

// share adjusts a match for what earlier rows of this batch already did.
func (b *batch) share(it Item, m Match) Match {
	if m.ByBarcode != nil && b.linked[*m.ByBarcode] {
		m.BarcodeLinked = true
	}
	if m.ByBarcode == nil && it.Barcode != nil {
		if id, ok := b.made[*it.Barcode]; ok {
			m.ByBarcode, m.BarcodeLinked = &id, true
		}
	}
	return m
}

func (b *batch) row(ctx context.Context, r sourceRow, m Match) error {
	it := r.item
	d := Decide(it, m, b.c.AutoCreateProducts)
	switch d.Action {
	case ActionUpdate:
		return b.update(ctx, r, m)
	case ActionCreate:
		return b.create(ctx, r, m)
	case ActionHold:
		return b.hold(ctx, r, m, d.Reason)
	default: // unmatched
		b.counts.Unmatched++
		return raiseIssue(ctx, b.tx, b.c, enum.SyncIssueKindUnmatched, it.ExternalID, payloadOf(it, d.Reason, m))
	}
}

func mrpOf(it Item, existing *Existing) *money.Cents {
	cents := it.MRPCents
	if cents == nil && existing != nil {
		cents = existing.MRPCents
	}
	if cents == nil {
		return nil
	}
	v := money.Cents(*cents)
	return &v
}

// priceRefused is a price the pricing rules cannot turn into a customer price.
func priceRefused(err error) bool {
	return errors.Is(err, pricing.ErrMRPBelowShopPrice) || errors.Is(err, pricing.ErrBadInput)
}

func (b *batch) update(ctx context.Context, r sourceRow, m Match) error {
	it := r.item
	applied, err := b.s.rt.Pricer.SetShopPrice(ctx, b.tx, db.System(), b.c.ShopID, m.Existing.ShopProductID,
		money.Cents(it.PriceCents), mrpOf(it, m.Existing), enum.PriceChangeSourceConnector)
	if priceRefused(err) {
		return b.hold(ctx, r, m, ReasonOverMRP)
	}
	if err != nil {
		return err
	}
	stockMoved, err := b.storeStock(ctx, m.Existing.ShopProductID, r, !applied.Changed)
	if err != nil {
		return err
	}
	if applied.Changed || stockMoved {
		b.counts.Changed++
	}
	b.applied = append(b.applied, it.ExternalID)
	return nil
}

func (b *batch) create(ctx context.Context, r sourceRow, m Match) error {
	it := r.item
	var productID uuid.UUID
	if m.ByBarcode != nil {
		productID = *m.ByBarcode
	} else {
		p, err := b.s.rt.Products.CreateProductIn(ctx, b.tx, db.System(), masterInput(it.Name, it.Barcode, it.Brand, it.ImageURL, it.Unit, it.WeightG))
		if err != nil {
			return err
		}
		productID = p.ID
		if it.Barcode != nil {
			b.made[*it.Barcode] = p.ID
		}
	}
	shopProductID, err := b.add(ctx, it, productID)
	if priceRefused(err) {
		return b.hold(ctx, r, m, ReasonOverMRP)
	}
	if err != nil {
		return err
	}
	b.linked[productID] = true
	if _, err := b.tx.Exec(ctx, `UPDATE shop_products SET source_hash = $2 WHERE id = $1`, shopProductID, r.hash); err != nil {
		return err
	}
	b.counts.Changed++
	b.applied = append(b.applied, it.ExternalID)
	return nil
}

// add puts a master product on sale in the shop under the row's external id.
func (b *batch) add(ctx context.Context, it Item, productID uuid.UUID) (uuid.UUID, error) {
	return b.s.addProduct(ctx, b.tx, db.System(), b.c.ShopID, it, productID, mrpOf(it, nil))
}

// addProduct creates the shop product with its first price. The sync and the
// issue actions both use it, so a product made by hand from an issue looks the
// same as one made by a run.
func (s *Service) addProduct(ctx context.Context, tx *db.Tx, scope db.Scope, shopID uuid.UUID, it Item, productID uuid.UUID, mrp *money.Cents) (uuid.UUID, error) {
	ext := it.ExternalID
	id, _, err := s.rt.Pricer.AddShopProduct(ctx, tx, scope, pricing.NewShopProduct{
		ShopID: shopID, ProductID: productID, ExternalID: &ext,
		ShopPrice: money.Cents(it.PriceCents), MRP: mrp, StockQty: stockOf(it), Available: availableOf(it),
		MaxPerOrder: defaultMaxPerOrder, ManagedBy: enum.ManagedByConnector, Source: enum.PriceChangeSourceConnector,
	})
	return id, err
}

// hold keeps the old price, raises PRICE_HOLD, and still takes the stock and
// availability of a product the shop already sells.
func (b *batch) hold(ctx context.Context, r sourceRow, m Match, reason string) error {
	if m.Existing != nil {
		if _, err := b.storeStock(ctx, m.Existing.ShopProductID, r, true); err != nil {
			return err
		}
	}
	b.counts.Held++
	return raiseIssue(ctx, b.tx, b.c, enum.SyncIssueKindPriceHold, r.item.ExternalID, payloadOf(r.item, reason, m))
}

// storeStock saves stock, availability and the row fingerprint, and marks the
// product as connector-managed. It reports whether stock or availability moved;
// when they did and the price did not (announce), customers' lists are told.
func (b *batch) storeStock(ctx context.Context, id uuid.UUID, r sourceRow, announce bool) (bool, error) {
	var moved bool
	err := b.tx.QueryRow(ctx, `
		UPDATE shop_products sp SET stock_qty = $2::numeric, is_available = $3, source_hash = $4, managed_by = 'CONNECTOR'
		FROM shop_products o WHERE sp.id = $1 AND o.id = sp.id
		RETURNING (o.stock_qty IS DISTINCT FROM sp.stock_qty OR o.is_available IS DISTINCT FROM sp.is_available)`,
		id, stockOf(r.item), availableOf(r.item), r.hash).Scan(&moved)
	if err != nil || !moved || !announce {
		return moved, err
	}
	return moved, events.AddCatalogChanged(ctx, b.tx, b.c.ShopID, []uuid.UUID{id})
}

// stockOf is the quantity to store: never negative, nil when the shop does not track stock.
func stockOf(it Item) *int64 {
	if it.Stock == nil {
		return nil
	}
	q := max(*it.Stock, 0)
	return &q
}

// availableOf: a product is on sale unless the source says it is not or the stock is used up.
func availableOf(it Item) bool {
	if it.Available != nil && !*it.Available {
		return false
	}
	return it.Stock == nil || *it.Stock > 0
}

func masterInput(name string, barcode, brand, imageURL *string, unit string, weightG *int) catalog.ProductInput {
	in := catalog.ProductInput{Name: &name}
	u := enum.Unit(unit)
	in.Unit = &u
	set := func(f *optional.Field[string], v *string) {
		if v != nil {
			*f = optional.Field[string]{Set: true, Value: *v}
		}
	}
	set(&in.Barcode, barcode)
	set(&in.Brand, brand)
	set(&in.ImageURL, imageURL)
	if weightG != nil {
		in.WeightG = optional.Field[int]{Set: true, Value: *weightG}
	}
	return in
}

// hideMissing makes the connector-managed products that the source no longer
// lists unavailable. The row fingerprint is dropped too, so the product comes
// back when its row does. Nothing is deleted. An empty source hides nothing: a table
// that is empty for a moment must not empty the shop.
func hideMissing(ctx context.Context, tx *db.Tx, shopID uuid.UUID, seen []string) (int, error) {
	if len(seen) == 0 {
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		UPDATE shop_products SET is_available = false, source_hash = NULL
		WHERE shop_id = $1 AND managed_by = 'CONNECTOR' AND is_available AND external_id IS NOT NULL AND external_id <> ALL($2::text[])
		RETURNING id`, shopID, seen)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(ids) == 0 {
		return 0, err
	}
	return len(ids), events.AddCatalogChanged(ctx, tx, shopID, ids)
}
