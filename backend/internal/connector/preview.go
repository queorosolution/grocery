package connector

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"gp/internal/connector/source"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
)

// PreviewRow is one source row as it would be read, and what a sync would do with it.
type PreviewRow struct {
	ExternalID string  `json:"externalId"`
	Name       string  `json:"name"`
	Barcode    *string `json:"barcode"`
	PriceCents *int64  `json:"priceCents"` // null when the price could not be read
	MRPCents   *int64  `json:"mrpCents"`
	StockQty   *int64  `json:"stockQty"`
	Available  *bool   `json:"available"`
	Action     Action  `json:"action"`
	Reason     string  `json:"reason,omitempty"`
}

// readSpec turns the saved mapping into a read: the mapped source columns, once
// each, ordered by the external id so the same rows come first every time.
func readSpec(c Connector, limit int) (spec source.ReadSpec, slot map[string]int, err error) {
	table, ok := source.ParseTable(deref(c.SourceTable))
	if !ok || c.Columns["externalId"] == "" {
		return spec, nil, &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"mapping": "not_set"}}
	}
	var cols []string
	index := map[string]int{} // source column -> position in cols
	slot = map[string]int{}   // our field -> position in cols
	keys := make([]string, 0, len(c.Columns))
	for k := range c.Columns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		col := c.Columns[k]
		i, seen := index[col]
		if !seen {
			i = len(cols)
			index[col] = i
			cols = append(cols, col)
		}
		slot[k] = i
	}
	return source.ReadSpec{Table: table, Columns: cols, OrderBy: c.Columns["externalId"], Limit: limit}, slot, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// fieldsOf picks our fields out of a source row.
func fieldsOf(row source.Row, slot map[string]int) map[string]*string {
	out := make(map[string]*string, len(slot))
	for field, i := range slot {
		out[field] = row[i]
	}
	return out
}

// Preview reads the first 20 rows through the saved mapping and says what a
// sync would do with each. It writes nothing to either database.
func (s *Service) Preview(ctx context.Context, scope db.Scope, id uuid.UUID) ([]PreviewRow, error) {
	rows := []PreviewRow{}
	err := s.with(ctx, scope, id, "preview", func(ctx context.Context, c Connector, conn source.Conn) error {
		spec, slot, err := readSpec(c, previewRows)
		if err != nil {
			return err
		}
		type read struct {
			item Item
			bad  Invalid
		}
		var reads []read
		err = conn.Read(ctx, spec, func(row source.Row) error {
			it, bad := Normalise(fieldsOf(row, slot), c.MoneyUnit, s.imgBase)
			reads = append(reads, read{it, bad})
			return nil
		})
		if err != nil {
			return err
		}
		var valid []Item
		for _, r := range reads {
			if r.bad == "" {
				valid = append(valid, r.item)
			}
		}
		var matches map[string]Match
		err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
			matches, err = loadMatches(ctx, tx, c.ShopID, valid)
			return err
		})
		if err != nil {
			return err
		}
		for _, r := range reads {
			rows = append(rows, previewRow(r.item, r.bad, matches[r.item.ExternalID], c.AutoCreateProducts))
		}
		return nil
	})
	return rows, err
}

func previewRow(it Item, bad Invalid, m Match, autoCreate bool) PreviewRow {
	row := PreviewRow{ExternalID: it.ExternalID, Name: it.Name}
	if bad != "" {
		row.Action, row.Reason = ActionInvalid, string(bad)
		return row
	}
	d := Decide(it, m, autoCreate)
	row.Action, row.Reason = d.Action, d.Reason
	row.Barcode, row.MRPCents, row.StockQty, row.Available = it.Barcode, it.MRPCents, it.Stock, it.Available
	if !it.PriceBad {
		row.PriceCents = &it.PriceCents
	}
	return row
}
