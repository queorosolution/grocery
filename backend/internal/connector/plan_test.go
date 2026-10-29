package connector

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
)

func ptr[T any](v T) *T { return &v }

func TestDecide(t *testing.T) {
	product := uuid.New()
	item := func(price int64) Item { return Item{ExternalID: "1", Name: "Rice", PriceCents: price} }
	withBarcode := func(price int64) Item { it := item(price); it.Barcode = ptr("4791012345678"); return it }
	existing := func(price int64, mrp *int64) Match {
		return Match{Existing: &Existing{ShopPriceCents: price, MRPCents: mrp}}
	}

	tests := []struct {
		name string
		it   Item
		m    Match
		auto bool
		want Decision
	}{
		{"saved link", item(1000), existing(1000, nil), false, Decision{ActionUpdate, ""}},
		{"saved link wins over the barcode", withBarcode(1000), Match{Existing: &Existing{ShopPriceCents: 1000}, ByBarcode: &product}, false, Decision{ActionUpdate, ""}},
		{"barcode match", withBarcode(1000), Match{ByBarcode: &product}, false, Decision{ActionCreate, ReasonBarcode}},
		{"barcode match, product already sold under another id", withBarcode(1000), Match{ByBarcode: &product, BarcodeLinked: true}, true, Decision{ActionUnmatched, ReasonAlreadyLinked}},
		{"no match, auto create", withBarcode(1000), Match{}, true, Decision{ActionCreate, ReasonAutoCreate}},
		{"no match, auto create without barcode", item(1000), Match{}, true, Decision{ActionCreate, ReasonAutoCreate}},
		{"no match, no barcode", item(1000), Match{}, false, Decision{ActionUnmatched, ReasonNoBarcode}},
		{"no match, unknown barcode", withBarcode(1000), Match{}, false, Decision{ActionUnmatched, ReasonUnknownBarcode}},

		{"zero price", item(0), existing(1000, nil), false, Decision{ActionHold, ReasonZeroPrice}},
		{"unreadable price", Item{ExternalID: "1", Name: "Rice", PriceBad: true}, existing(1000, nil), false, Decision{ActionHold, ReasonBadPrice}},
		{"over the source MRP", Item{ExternalID: "1", Name: "Rice", PriceCents: 1200, MRPCents: ptr(int64(1100))}, existing(1000, nil), false, Decision{ActionHold, ReasonOverMRP}},
		{"over the stored MRP", item(1200), existing(1000, ptr(int64(1100))), false, Decision{ActionHold, ReasonOverMRP}},
		{"at the MRP", item(1100), existing(1000, ptr(int64(1100))), false, Decision{ActionUpdate, ""}},
		{"drop of more than half", item(499), existing(1000, nil), false, Decision{ActionHold, ReasonBigDrop}},
		{"drop of exactly half", item(500), existing(1000, nil), false, Decision{ActionUpdate, ""}},
		{"a rise is fine", item(5000), existing(1000, nil), false, Decision{ActionUpdate, ""}},
		{"a new item has no drop to measure", item(1), Match{ByBarcode: &product}, false, Decision{ActionCreate, ReasonBarcode}},
		{"a new item can still be held", item(0), Match{ByBarcode: &product}, false, Decision{ActionHold, ReasonZeroPrice}},
		{"an unmatched item is not judged on price", item(0), Match{}, false, Decision{ActionUnmatched, ReasonNoBarcode}},
	}
	for _, tc := range tests {
		if got := Decide(tc.it, tc.m, tc.auto); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestCleanMapping(t *testing.T) {
	good := MappingInput{SourceTable: "gp_products", Columns: map[string]string{"externalId": "external_id", "name": "name", "price": "price", "barcode": " barcode "}}
	in, table, v := cleanMapping(good)
	if err := v.Err(); err != nil {
		t.Fatal(err)
	}
	if in.SourceTable != "public.gp_products" || table.Name != "gp_products" || in.MoneyUnit != "RUPEES" || in.IntervalMin != 60 || in.Columns["barcode"] != "barcode" {
		t.Errorf("defaults not applied: %+v", in)
	}

	bad := map[string]MappingInput{
		"sourceTable":        {SourceTable: `x"; DROP TABLE y;--`, Columns: good.Columns},
		"moneyUnit":          {SourceTable: "t", MoneyUnit: "EUROS", Columns: good.Columns},
		"intervalMin":        {SourceTable: "t", IntervalMin: 5, Columns: good.Columns},
		"columns.price":      {SourceTable: "t", Columns: map[string]string{"externalId": "a", "name": "b"}},
		"columns.bogus":      {SourceTable: "t", Columns: map[string]string{"externalId": "a", "name": "b", "price": "c", "bogus": "d"}},
		"columns.name":       {SourceTable: "t", Columns: map[string]string{"externalId": "a", "name": "Name", "price": "c"}},
		"columns.externalId": {SourceTable: "t", Columns: map[string]string{"externalId": "", "name": "b", "price": "c"}},
		"changeColumn":       {SourceTable: "t", ChangeColumn: ptr("x y"), Columns: good.Columns},
	}
	for field, m := range bad {
		_, _, v := cleanMapping(m)
		var app *apperr.Error
		if err := v.Err(); !errors.As(err, &app) || app.Fields[field] == "" {
			t.Errorf("%s: not flagged (err = %v)", field, err)
		}
	}
}
