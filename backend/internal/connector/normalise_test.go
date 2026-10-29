package connector

import (
	"testing"

	"gp/internal/shared/enum"
)

func p(s string) *string { return &s }

func TestParseMoney(t *testing.T) {
	tests := []struct {
		in   string
		unit enum.MoneyUnit
		want int64
		ok   bool
	}{
		{"1250", enum.MoneyUnitRupees, 125000, true},
		{"1250.5", enum.MoneyUnitRupees, 125050, true},
		{"1250.50", enum.MoneyUnitRupees, 125050, true},
		{"480.505", enum.MoneyUnitRupees, 48051, true}, // half up
		{"480.504", enum.MoneyUnitRupees, 48050, true},
		{"99.995", enum.MoneyUnitRupees, 10000, true}, // the carry goes into the rupees
		{"0", enum.MoneyUnitRupees, 0, true},
		{"0.00", enum.MoneyUnitRupees, 0, true},
		{" 12 ", enum.MoneyUnitRupees, 1200, true},
		{"125050", enum.MoneyUnitCents, 125050, true},
		{"125050.4", enum.MoneyUnitCents, 125050, true},
		{"125050.5", enum.MoneyUnitCents, 125051, true},
		{"", enum.MoneyUnitRupees, 0, false},
		{"abc", enum.MoneyUnitRupees, 0, false},
		{"-5", enum.MoneyUnitRupees, 0, false},
		{"+5", enum.MoneyUnitRupees, 0, false},
		{"1e3", enum.MoneyUnitRupees, 0, false},
		{"1,250.00", enum.MoneyUnitRupees, 0, false},
		{"1250.", enum.MoneyUnitRupees, 0, false},
		{".5", enum.MoneyUnitRupees, 0, false},
		{"1.2.3", enum.MoneyUnitRupees, 0, false},
		{"1250.5x", enum.MoneyUnitRupees, 0, false},
		{"NaN", enum.MoneyUnitRupees, 0, false},
		{"9999999999999999", enum.MoneyUnitRupees, 0, false}, // 16 digits
	}
	for _, tc := range tests {
		got, ok := ParseMoney(tc.in, tc.unit)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseMoney(%q, %s) = %d, %v; want %d, %v", tc.in, tc.unit, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalise(t *testing.T) {
	raw := map[string]*string{
		"externalId": p(" 4711 "), "name": p("  Samba Rice 5kg "), "price": p("1250.5"), "mrp": p("1300"),
		"barcode": p("4791-0123 45678"), "stock": p("12.000"), "available": p("t"), "unit": p("KGS"), "brand": p(" Araliya "),
		"weightG": p("5000"), "category": p("Rice"),
	}
	it, bad := Normalise(raw, enum.MoneyUnitRupees, "https://img.example.test")
	if bad != "" {
		t.Fatal(bad)
	}
	if it.ExternalID != "4711" || it.Name != "Samba Rice 5kg" || it.PriceCents != 125050 || *it.MRPCents != 130000 || it.PriceBad {
		t.Errorf("item = %+v", it)
	}
	if *it.Barcode != "4791012345678" || *it.Stock != 12 || !*it.Available || it.Unit != "KG" || *it.Brand != "Araliya" || *it.WeightG != 5000 || *it.Category != "Rice" {
		t.Errorf("fields = barcode %q stock %d available %v unit %s brand %s weight %d", *it.Barcode, *it.Stock, *it.Available, it.Unit, *it.Brand, *it.WeightG)
	}
}

func TestNormalise_Invalid(t *testing.T) {
	cases := map[string]struct {
		raw  map[string]*string
		want Invalid
	}{
		"no id":       {map[string]*string{"name": p("Rice"), "price": p("1")}, NoExternalID},
		"blank id":    {map[string]*string{"externalId": p("  "), "name": p("Rice")}, NoExternalID},
		"id too long": {map[string]*string{"externalId": p(string(make([]rune, 101))), "name": p("Rice")}, NoExternalID},
		"no name":     {map[string]*string{"externalId": p("1"), "price": p("1")}, NoName},
		"one letter":  {map[string]*string{"externalId": p("1"), "name": p("R")}, NoName},
	}
	for name, tc := range cases {
		if _, bad := Normalise(tc.raw, enum.MoneyUnitRupees, ""); bad != tc.want {
			t.Errorf("%s: got %q, want %q", name, bad, tc.want)
		}
	}
}

func TestNormalise_PriceProblemsAreKeptNotDropped(t *testing.T) {
	for name, price := range map[string]*string{"missing": nil, "text": p("free"), "negative": p("-1"), "over the cap": p("5000001")} {
		it, bad := Normalise(map[string]*string{"externalId": p("1"), "name": p("Rice"), "price": price}, enum.MoneyUnitRupees, "")
		if bad != "" || !it.PriceBad || it.PriceCents != 0 {
			t.Errorf("%s: item = %+v, invalid = %q; want PriceBad and no invalid row", name, it, bad)
		}
	}
	if it, _ := Normalise(map[string]*string{"externalId": p("1"), "name": p("Rice"), "price": p("5000000")}, enum.MoneyUnitRupees, ""); it.PriceBad {
		t.Error("exactly the cap must be allowed")
	}
}

func TestNormalise_OptionalFieldsAreForgiving(t *testing.T) {
	it, _ := Normalise(map[string]*string{
		"externalId": p("1"), "name": p("Rice"), "price": p("10"),
		"barcode": p("12ab"), "stock": p("lots"), "available": p("maybe"), "unit": p("bunch"), "weightG": p("0"),
		"mrp": p("-3"), "imageUrl": p("https://evil.example/x.jpg"),
	}, enum.MoneyUnitRupees, "https://img.example.test")
	if it.Barcode != nil || it.Stock != nil || it.Available != nil || it.Unit != "PCS" || it.WeightG != nil || it.MRPCents != nil || it.ImageURL != nil {
		t.Errorf("junk should become 'not set': %+v", it)
	}
	ok, _ := Normalise(map[string]*string{"externalId": p("1"), "name": p("Rice"), "price": p("10"), "imageUrl": p("https://img.example.test/a/b.webp")}, enum.MoneyUnitRupees, "https://img.example.test")
	if ok.ImageURL == nil {
		t.Error("an image under the image host should be kept")
	}
	for in, want := range map[string]bool{"t": true, "TRUE": true, "1": true, "Yes": true, "f": false, "false": false, "0": false, "N": false} {
		if got := boolean(p(in)); got == nil || *got != want {
			t.Errorf("boolean(%q) = %v, want %v", in, got, want)
		}
	}
	for in, want := range map[string]int64{"12": 12, "0": 0, "-3": -3, "-3.5": -3, "12.999": 12} {
		if got := wholeNumber(p(in)); got == nil || *got != want {
			t.Errorf("wholeNumber(%q) = %v, want %d", in, got, want)
		}
	}
}

func TestNormalise_LongTextIsCutNotRefused(t *testing.T) {
	long := make([]rune, 300)
	for i := range long {
		long[i] = 'ස' // multi-byte, to cut on a character and not a byte
	}
	it, bad := Normalise(map[string]*string{"externalId": p("1"), "name": p(string(long)), "price": p("1"), "brand": p(string(long))}, enum.MoneyUnitRupees, "")
	if bad != "" || len([]rune(it.Name)) != 120 || len([]rune(*it.Brand)) != 60 {
		t.Errorf("name %d runes, brand %d runes, invalid %q", len([]rune(it.Name)), len([]rune(*it.Brand)), bad)
	}
}
