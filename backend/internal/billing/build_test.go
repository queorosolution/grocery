package billing

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
	"gp/internal/shared/geo"
	"gp/internal/shared/money"
)

var (
	colombo = geo.Point{Lat: 6.9271, Lng: 79.8612}
	near    = geo.Point{Lat: 6.9487, Lng: 79.8612} // about 2.4 km north
	now     = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)

	rice  = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	sugar = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	soap  = uuid.MustParse("00000000-0000-4000-8000-000000000003")
)

func products() map[uuid.UUID]ProductSnapshot {
	return map[uuid.UUID]ProductSnapshot{
		rice:  {ID: rice, Name: "Samba Rice 5kg", Unit: "PCS", ShopPriceCents: 140000, OnlinePriceCents: 143500, Available: true, MaxPerOrder: 5, WeightG: 5000},
		sugar: {ID: sugar, Name: "Sugar 1kg", Unit: "PCS", ShopPriceCents: 30000, OnlinePriceCents: 31000, Available: true, MaxPerOrder: 20, WeightG: 1000},
		soap:  {ID: soap, Name: "Soap", Unit: "PCS", ShopPriceCents: 10000, OnlinePriceCents: 10000, Available: false, MaxPerOrder: 20, WeightG: 100},
	}
}

func input(lines ...LineInput) BillInput {
	return BillInput{
		Shop:     ShopSnapshot{Location: colombo, MinOrderCents: 100000, MaxLines: 60, Open: true},
		Lines:    lines,
		Products: products(),
		Address:  near,
		FeeRule:  delivery.Rule{BaseCents: 15000, MaxKm: 1000, RoadFactor: 130}, FeeRuleSource: "SHOP",
		PhoneVerified: true, Now: now,
	}
}

func line(id uuid.UUID, qty int) LineInput { return LineInput{ShopProductID: id, Qty: qty} }

func code(err error) apperr.Code {
	var app *apperr.Error
	if errors.As(err, &app) {
		return app.Code
	}
	return ""
}

func TestBuild_Totals(t *testing.T) {
	in := input(line(rice, 1), line(sugar, 2))
	got, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	want := Totals{ItemsCents: 143500 + 2*31000, DeliveryFeeCents: 15000}
	want.TotalCents = want.ItemsCents + want.DeliveryFeeCents
	if got.Totals != want {
		t.Errorf("totals = %+v, want %+v", got.Totals, want)
	}
	if got.MarginCents != 3500+2*1000 {
		t.Errorf("margin = %d", got.MarginCents)
	}
	if len(got.Lines) != 2 || got.Lines[1].LineCents != 62000 || got.Lines[0].Status != StatusOK {
		t.Errorf("lines = %+v", got.Lines)
	}
	if got.Delivery.WeightKg != 700 || got.Delivery.FeeRule != "SHOP" || got.Delivery.DistanceKm < 230 || got.Delivery.DistanceKm > 250 {
		t.Errorf("delivery = %+v", got.Delivery)
	}
}

func TestBuild_ServiceFeeMakesUpTheMinimumMargin(t *testing.T) {
	for name, tc := range map[string]struct {
		min  money.Cents
		want money.Cents
	}{
		"no minimum":               {0, 0},
		"margin already enough":    {5500, 0},
		"margin just short":        {5501, 1},
		"margin far short":         {20000, 14500},
		"minimum below the margin": {100, 0},
	} {
		in := input(line(rice, 1), line(sugar, 2)) // margin 5500
		in.Shop.MinPlatformMarginCents = tc.min
		got, err := Build(in)
		if err != nil || got.Totals.ServiceFeeCents != tc.want {
			t.Errorf("%s: service fee %d (%v), want %d", name, got.Totals.ServiceFeeCents, err, tc.want)
		}
		if sum := got.Totals.ItemsCents + got.Totals.DeliveryFeeCents + got.Totals.ServiceFeeCents; got.Totals.TotalCents != sum {
			t.Errorf("%s: total %d is not the sum %d", name, got.Totals.TotalCents, sum)
		}
	}
}

// BIZ-01: the bill uses the database price, whatever the cart says.
func TestBuild_PricesComeFromTheProducts(t *testing.T) {
	cheap, same := money.Cents(1), money.Cents(31000)
	l1, l2 := line(rice, 1), line(sugar, 1)
	l1.CartUnitPrice, l2.CartUnitPrice = &cheap, &same
	got, err := Build(input(l1, l2))
	if err != nil {
		t.Fatal(err)
	}
	if got.Lines[0].UnitPriceCents != 143500 || !got.Lines[0].PriceChanged {
		t.Errorf("rice = %+v, want the database price and a price-changed flag", got.Lines[0])
	}
	if got.Lines[1].PriceChanged {
		t.Error("sugar price did not change")
	}
	if got.Totals.ItemsCents != 143500+31000 {
		t.Errorf("items = %d, want the database prices added", got.Totals.ItemsCents)
	}
}

// BIZ-02: quantities are whole numbers from 1 to the product's limit.
func TestBuild_Quantities(t *testing.T) {
	for name, qty := range map[string]int{"zero": 0, "negative": -1, "huge": 1_000_000, "over the product limit": 6, "just over absolute": 1000} {
		if _, err := Build(input(line(rice, qty))); code(err) != apperr.QtyLimit {
			t.Errorf("%s: error = %v, want QTY_LIMIT", name, err)
		}
	}
	for _, qty := range []int{1, 5} {
		if _, err := Build(input(line(rice, qty))); err != nil {
			t.Errorf("qty %d: %v", qty, err)
		}
	}
	// A bad quantity is refused even on a product that is not for sale.
	if _, err := Build(input(line(soap, 0))); code(err) != apperr.QtyLimit {
		t.Errorf("zero of an unavailable product: %v", err)
	}
}

func TestBuild_ExcludesWhatIsNotForSale(t *testing.T) {
	unknown := uuid.MustParse("00000000-0000-4000-8000-0000000000ff")
	got, err := Build(input(line(rice, 1), line(soap, 3), line(unknown, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 1 || len(got.Excluded) != 2 || got.Excluded[0].Reason != ReasonItemUnavailable || got.Excluded[0].Name != "Soap" {
		t.Errorf("lines %d, excluded %+v", len(got.Lines), got.Excluded)
	}
	if got.Totals.ItemsCents != 143500 {
		t.Errorf("items = %d: an excluded line must not be added", got.Totals.ItemsCents)
	}

	none, err := Build(input(line(soap, 1)))
	if err != nil || none.Totals != (Totals{}) || len(none.Lines) != 0 || len(none.Excluded) != 1 {
		t.Errorf("only unavailable lines: %+v %v, want an empty bill with no fees", none, err)
	}
}

func TestBuild_RefusesABadRequest(t *testing.T) {
	many := input()
	many.Shop.MaxLines = 2
	many.Lines = []LineInput{line(rice, 1), line(sugar, 1), line(soap, 1)}
	bad := line(rice, 1)
	bad.SubPref = "NEVER"
	for name, tc := range map[string]struct {
		in   BillInput
		want apperr.Code
	}{
		"no lines":        {input(), apperr.CartEmpty},
		"too many lines":  {many, apperr.QtyLimit},
		"same line twice": {input(line(rice, 1), line(rice, 2)), apperr.ValidationError},
		"bad choice":      {input(bad), apperr.ValidationError},
	} {
		if _, err := Build(tc.in); code(err) != tc.want {
			t.Errorf("%s: error = %v, want %s", name, err, tc.want)
		}
	}
	got, err := Build(input(line(rice, 1)))
	if err != nil || got.Lines[0].SubPref != enum.SubstitutionPrefSimilar {
		t.Errorf("a line with no choice should default to SIMILAR: %+v %v", got.Lines, err)
	}
}

func TestBuild_Checks(t *testing.T) {
	in := input(line(sugar, 1)) // 31000 is below the 100000 minimum
	in.Shop.Open, in.PhoneVerified = false, false
	got, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Checks != (Checks{ShopOpen: false, InRange: true, MinOrderMet: false, PhoneVerified: false}) {
		t.Errorf("checks = %+v", got.Checks)
	}
	if got.Totals.DeliveryFeeCents != 15000 {
		t.Errorf("a bill below the minimum still shows its fee: %+v", got.Totals)
	}

	// Exactly the minimum counts as met.
	in = input(line(sugar, 1))
	in.Shop.MinOrderCents = 31000
	if got, _ = Build(in); !got.Checks.MinOrderMet {
		t.Error("an order equal to the minimum must be accepted")
	}
}

func TestBuild_OutOfRangeShowsTheBillWithoutAFee(t *testing.T) {
	in := input(line(rice, 1))
	in.FeeRule.MaxKm = 100 // 1 km
	got, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Checks.InRange || got.Totals.DeliveryFeeCents != 0 || got.Totals.TotalCents != 143500 {
		t.Errorf("out of range = %+v", got)
	}
}

func TestBuild_DeliveryFeeIsTheFeeFormula(t *testing.T) {
	in := input(line(rice, 2), line(sugar, 1)) // 11 kg, items 318000
	in.FeeRule = delivery.Rule{BaseCents: 10000, PerKmCents: 2500, FreeKm: 100, PerKgCents: 500, FreeKg: 500, MaxKm: 1000, RoadFactor: 130}
	free := money.Cents(300000)
	got, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	want, err := delivery.Fee(in.FeeRule, geo.HaversineM(colombo, near), 11000, 318000)
	if err != nil || got.Totals.DeliveryFeeCents != want || want <= 10000 {
		t.Errorf("fee = %d, want %d (%v)", got.Totals.DeliveryFeeCents, want, err)
	}
	in.FeeRule.FreeAboveCents = &free
	if got, _ = Build(in); got.Totals.DeliveryFeeCents != 0 {
		t.Errorf("delivery should be free above the limit, got %d", got.Totals.DeliveryFeeCents)
	}
}
