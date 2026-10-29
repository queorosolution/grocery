package delivery

import (
	"encoding/json"
	"testing"

	"gp/internal/shared/apperr"
	"gp/internal/shared/money"
)

func cents(c int64) *money.Cents { v := money.Cents(c); return &v }

// exampleRule is the rule from docs/07 section 7: base Rs 100, Rs 30 per km
// after 2 km, Rs 10 per kg after 5 kg.
func exampleRule() Rule {
	return Rule{BaseCents: 10000, PerKmCents: 3000, FreeKm: 200, PerKgCents: 1000, FreeKg: 500, MaxKm: 800, RoadFactor: 130}
}

func TestFee_DocExample(t *testing.T) {
	// 2.0 km straight x 1.3 = 2.6 km, 7.5 kg: 100 + ceil(0.6) x 30 + ceil(2.5) x 10 = Rs 160.
	got, err := Fee(exampleRule(), 2000, 7500, 0)
	if err != nil || got != 16000 {
		t.Fatalf("Fee = %d, %v; want 16000", got, err)
	}
}

func TestFee_Table(t *testing.T) {
	free := func(r Rule) Rule { r.FreeAboveCents = cents(500000); return r }
	clamp := func(r Rule) Rule { r.MinCents = 20000; r.MaxCents = cents(25000); return r }
	tests := []struct {
		name      string
		rule      Rule
		distanceM int
		weightG   int
		items     money.Cents
		want      money.Cents
	}{
		{"inside free km and kg: base only", exampleRule(), 1000, 1000, 0, 10000},
		{"road km exactly at free km: no extra", exampleRule(), 1538, 5000, 0, 10000}, // 1538 x 1.3 = 1999.4 m
		{"one metre past free km: one extra km", exampleRule(), 1539, 5000, 0, 13000}, // 2000.7 m road
		{"weight exactly at free kg: no extra", exampleRule(), 0, 5000, 0, 10000},
		{"one gram over: one extra kg", exampleRule(), 0, 5001, 0, 11000},
		{"zero distance and weight", exampleRule(), 0, 0, 0, 10000},
		{"free above: exactly the limit", free(exampleRule()), 2000, 7500, 500000, 0},
		{"free above: one cent short", free(exampleRule()), 2000, 7500, 499999, 16000},
		{"min clamps up", clamp(exampleRule()), 0, 0, 0, 20000},
		{"max clamps down", clamp(exampleRule()), 6000, 30000, 0, 25000},
		{"between min and max is untouched", clamp(exampleRule()), 2000, 5000, 0, 20000},
		{"exactly at max km", exampleRule(), 6153, 0, 0, 10000 + 6*3000}, // 7998.9 m road: 6 extra km
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Fee(tc.rule, tc.distanceM, tc.weightG, tc.items)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Fee = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFee_FreeDeliverySkipsMinimum(t *testing.T) {
	r := exampleRule()
	r.MinCents = 20000
	r.FreeAboveCents = cents(100000)
	got, err := Fee(r, 1000, 1000, 100000)
	if err != nil || got != 0 {
		t.Fatalf("free delivery must stay 0, got %d, %v", got, err)
	}
}

func TestFee_OutOfRange(t *testing.T) {
	r := exampleRule() // 8.00 km road
	r.FreeAboveCents = cents(1)
	// 6154 m x 1.3 = 8000.2 m road: just over. Out of range wins over free delivery.
	if _, err := Fee(r, 6154, 0, 999999); !apperr.Is(err, apperr.OutOfRange) {
		t.Fatalf("got %v, want OUT_OF_RANGE", err)
	}
	if _, err := Fee(exampleRule(), 6153, 0, 0); err != nil { // 7998.9 m road: inside
		t.Fatalf("inside range: %v", err)
	}
}

func TestFee_RejectsBadInput(t *testing.T) {
	for _, tc := range []struct{ d, w int }{{-1, 0}, {0, -1}} {
		if _, err := Fee(exampleRule(), tc.d, tc.w, 0); !apperr.Is(err, apperr.ValidationError) {
			t.Errorf("Fee(%d, %d): %v, want VALIDATION_ERROR", tc.d, tc.w, err)
		}
	}
}

func TestFee_OverflowIsAnError(t *testing.T) {
	r := Rule{PerKgCents: 1 << 62, FreeKg: 0, MaxKm: 5000, RoadFactor: 100}
	if _, err := Fee(r, 0, 1_000_000_000, 0); err == nil {
		t.Fatal("overflow must be reported, not wrapped")
	}
}

func TestRuleValidate(t *testing.T) {
	bad := map[string]func(*Rule){
		"negative base":       func(r *Rule) { r.BaseCents = -1 },
		"max below min":       func(r *Rule) { r.MinCents = 500; r.MaxCents = cents(100) },
		"free above zero":     func(r *Rule) { r.FreeAboveCents = cents(0) },
		"max km zero":         func(r *Rule) { r.MaxKm = 0 },
		"max km over 50":      func(r *Rule) { r.MaxKm = 5001 },
		"road factor below 1": func(r *Rule) { r.RoadFactor = 99 },
		"road factor above 2": func(r *Rule) { r.RoadFactor = 201 },
		"absurd money":        func(r *Rule) { r.PerKmCents = maxMoney + 1 },
	}
	if err := exampleRule().Validate(); err != nil {
		t.Fatalf("example rule: %v", err)
	}
	for name, mutate := range bad {
		r := exampleRule()
		mutate(&r)
		if err := r.Validate(); !apperr.Is(err, apperr.ValidationError) {
			t.Errorf("%s: %v, want VALIDATION_ERROR", name, err)
		}
	}
}

func TestDec2JSON(t *testing.T) {
	var ok struct{ A, B, C Dec2 }
	if err := json.Unmarshal([]byte(`{"A":2,"B":2.5,"C":0.07}`), &ok); err != nil || ok.A != 200 || ok.B != 250 || ok.C != 7 {
		t.Fatalf("parse: %+v, %v", ok, err)
	}
	out, _ := json.Marshal(ok)
	if string(out) != `{"A":2.00,"B":2.50,"C":0.07}` {
		t.Errorf("marshal = %s", out)
	}
	for _, in := range []string{`"2"`, `-1`, `2.555`, `1e3`, `null`, `""`, `1234567`, `.5`} {
		var d Dec2
		if err := json.Unmarshal([]byte(in), &d); err == nil && in != `null` {
			t.Errorf("%s accepted as %d", in, d)
		}
	}
}
