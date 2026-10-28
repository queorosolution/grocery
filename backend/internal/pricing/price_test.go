package pricing_test

import (
	"errors"
	"math"
	"testing"

	"gp/internal/pricing"
	"gp/internal/shared/money"
)

func mrp(c money.Cents) *money.Cents { return &c }

func TestOnlinePrice_Table(t *testing.T) {
	tests := []struct {
		name  string
		shop  money.Cents
		mrp   *money.Cents
		bp    int
		want  money.Cents
		wantE error
	}{
		{"spec example", 140000, mrp(150000), 250, 143500, nil},
		{"rounds half up to the rupee", 100000, nil, 5, 100100, nil}, // raw 100050
		{"rounds down under half", 100000, nil, 4, 100000, nil},      // raw 100040
		{"no markup keeps a whole-rupee price", 45000, nil, 0, 45000, nil},
		{"capped at the MRP", 140000, mrp(142000), 250, 142000, nil},
		{"MRP equal to shop price", 140000, mrp(140000), 250, 140000, nil},
		{"MRP below shop price", 140000, mrp(130000), 250, 0, pricing.ErrMRPBelowShopPrice},
		{"rounding falls under the shop price", 10040, nil, 0, 0, pricing.ErrMRPBelowShopPrice},
		{"largest markup", 100000, nil, 2000, 120000, nil},
		{"zero price", 0, nil, 250, 0, pricing.ErrBadInput},
		{"negative price", -1, nil, 250, 0, pricing.ErrBadInput},
		{"markup too high", 100000, nil, 2001, 0, pricing.ErrBadInput},
		{"negative markup", 100000, nil, -1, 0, pricing.ErrBadInput},
		{"zero MRP", 100000, mrp(0), 250, 0, pricing.ErrBadInput},
		{"overflow", money.Cents(math.MaxInt64 - 5), nil, 2000, 0, money.ErrOverflow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pricing.OnlinePrice(tc.shop, tc.mrp, tc.bp)
			if !errors.Is(err, tc.wantE) || got != tc.want {
				t.Fatalf("OnlinePrice = %d, %v; want %d, %v", got, err, tc.want, tc.wantE)
			}
		})
	}
}

func TestOnlinePrice_NeverBelowShopPriceOrAboveMRP(t *testing.T) {
	for shop := money.Cents(100); shop <= 20000; shop += 137 {
		for _, bp := range []int{0, 1, 250, 999, 2000} {
			ceiling := shop + 5000
			got, err := pricing.OnlinePrice(shop, &ceiling, bp)
			if err != nil {
				continue // rounding down under the shop price is an allowed error
			}
			if got < shop || got > ceiling || got%100 != 0 && got != ceiling {
				t.Fatalf("shop %d bp %d -> %d (ceiling %d)", shop, bp, got, ceiling)
			}
		}
	}
}
