package connector

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/connector/source"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// timeTypes are the column types that can drive an incremental sync.
var timeTypes = map[string]bool{"timestamp with time zone": true, "timestamp without time zone": true, "date": true}

// SetMapping validates the mapping against the shop's database and saves it.
// Every table and column name must be a plain lower-case name AND exist in
// information_schema, so no text from the request ever reaches SQL (SQLI-02).
// It resets the sync cursor: the next run reads everything again.
func (s *Service) SetMapping(ctx context.Context, scope db.Scope, id uuid.UUID, in MappingInput) (Connector, error) {
	in, table, v := cleanMapping(in)
	if err := v.Err(); err != nil {
		return Connector{}, err
	}
	var changeCol *string
	err := s.with(ctx, scope, id, "mapping", func(ctx context.Context, _ Connector, conn source.Conn) error {
		cols, err := conn.Columns(ctx, table)
		if err != nil {
			return err
		}
		var v2 *validate.Validator
		v2, changeCol = checkAgainstSource(in, cols)
		return v2.Err()
	})
	if err != nil {
		return Connector{}, err
	}
	var out Connector
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := byID(ctx, tx, id, true)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if out, err = saveMapping(ctx, tx, id, in, changeCol); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "connector.mapping.set", Entity: "connector", EntityID: id.String(), ShopID: before.ShopID,
			Before: mappingOf(before), After: mappingOf(out),
		})
	})
	return out, err
}

// mappingOf is the part of a connector the audit row records.
func mappingOf(c Connector) map[string]any {
	return map[string]any{
		"sourceTable": c.SourceTable, "moneyUnit": c.MoneyUnit, "changeColumn": c.ChangeColumn,
		"intervalMin": c.IntervalMin, "autoCreateProducts": c.AutoCreateProducts, "columns": c.Columns,
	}
}

// cleanMapping applies defaults and checks everything that needs no database.
func cleanMapping(in MappingInput) (MappingInput, source.TableName, *validate.Validator) {
	v := validate.New()
	if in.MoneyUnit == "" {
		in.MoneyUnit = enum.MoneyUnitRupees
	}
	if in.IntervalMin == 0 {
		in.IntervalMin = defaultIntervalMin
	}
	table, ok := source.ParseTable(strings.TrimSpace(in.SourceTable))
	v.Check("sourceTable", ok, "invalid")
	if ok {
		in.SourceTable = table.String()
	}
	v.Check("moneyUnit", in.MoneyUnit.Valid(), "invalid").
		Check("intervalMin", in.IntervalMin >= minIntervalMin && in.IntervalMin <= maxIntervalMin, "out_of_range")
	if in.ChangeColumn != nil {
		c := strings.TrimSpace(*in.ChangeColumn)
		if c == "" {
			in.ChangeColumn = nil
		} else {
			in.ChangeColumn = &c
			v.Check("changeColumn", source.IsIdent(c), "invalid")
		}
	}
	cols := make(map[string]string, len(in.Columns))
	for key, col := range in.Columns {
		col = strings.TrimSpace(col)
		switch {
		case !isField(key):
			v.Check("columns."+key, false, "unknown_field")
		case col == "": // an empty value means "not mapped"
		default:
			v.Check("columns."+key, source.IsIdent(col), "invalid")
			cols[key] = col
		}
	}
	in.Columns = cols
	for _, f := range Fields {
		if f.Required {
			v.Check("columns."+f.Key, cols[f.Key] != "", "required")
		}
	}
	return in, table, v
}

// checkAgainstSource compares the mapping with the real columns. It returns the
// change column to store: the explicit one, else the one mapped as updatedAt.
func checkAgainstSource(in MappingInput, cols []source.Column) (*validate.Validator, *string) {
	v := validate.New()
	types := make(map[string]string, len(cols))
	for _, c := range cols {
		types[c.Name] = c.Type
	}
	v.Check("sourceTable", len(cols) > 0, "unknown")
	for key, col := range in.Columns {
		_, ok := types[col]
		v.Check("columns."+key, ok || len(cols) == 0, "unknown_column")
	}
	change := in.ChangeColumn
	if change == nil {
		if u, ok := in.Columns["updatedAt"]; ok {
			change = &u
		}
	} else if u, ok := in.Columns["updatedAt"]; ok && u != *change {
		v.Check("changeColumn", false, "differs_from_updatedAt")
	}
	if change != nil {
		t, ok := types[*change]
		v.Check("changeColumn", ok || len(cols) == 0, "unknown_column").Check("changeColumn", !ok || timeTypes[t], "not_a_time_column")
	}
	return v, change
}
