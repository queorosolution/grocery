package catalog_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"gp/internal/catalog"
)

func search(f fixture, query string) catalog.SearchResult {
	f.a.T.Helper()
	return decode[catalog.SearchResult](f, f.publicGet("/search?"+query))
}

func names(r catalog.SearchResult) []string {
	out := []string{}
	for _, p := range r.Products.Data {
		out = append(out, p.Name)
	}
	return out
}

func searchWorld(t *testing.T) (f fixture, near, other uuid.UUID) {
	t.Helper()
	f = newFixture(t)
	f.admin.OK(http.MethodPut, "/admin/fee-rules/default", docRule)
	near = f.activeShop("near-shop", 6.9271, 79.8612)
	other = f.activeShop("rice-house", 6.9300, 79.8650)
	far := f.activeShop("far-shop", 7.2906, 80.6337)
	f.stock(near, "Basmati Rice 1kg", 52000, nil, true, nil)
	f.stock(other, "Basmati Rice 1kg", 48000, nil, true, nil)
	f.stock(far, "Basmati Rice 1kg", 40000, nil, true, nil)
	f.stock(near, "Tomato", 30000, nil, true, nil)
	f.stock(near, "Coconut Oil 500ml", 90000, nil, true, nil)
	return f, near, other
}

func TestSearch_FindsTyposAndWordsInsideNames(t *testing.T) {
	f, _, _ := searchWorld(t)
	for query, want := range map[string]string{
		"rice": "Basmati Rice 1kg", "RICE": "Basmati Rice 1kg", "basmti": "Basmati Rice 1kg", "tomatto": "Tomato",
		"cocnut": "Coconut Oil 500ml", "brand": "Basmati Rice 1kg",
	} {
		if got := names(search(f, "q="+url.QueryEscape(query))); len(got) == 0 || got[0] != want && query != "brand" {
			t.Errorf("q=%q found %v, want %q", query, got, want)
		}
	}
	if got := names(search(f, "q=zzzzqq")); len(got) != 0 {
		t.Errorf("nonsense found %v", got)
	}
}

// SQLI-03: the text is a parameter; odd text is just an empty result.
func TestSearch_SQLInjectionText(t *testing.T) {
	f, _, _ := searchWorld(t)
	for _, q := range []string{`%' OR 1=1 --`, `'; DROP TABLE products; --`, `" OR ""="`, `rice%`, `\\`, `_ _`} {
		w := f.publicGet("/search?q=" + url.QueryEscape(q))
		if w.Code != http.StatusOK {
			t.Errorf("q=%q = %d %s", q, w.Code, w.Body)
			continue
		}
		if got := names(decode[catalog.SearchResult](f, w)); len(got) > 0 && q != `rice%` {
			t.Errorf("q=%q matched %v", q, got)
		}
	}
	if n := f.a.Count(`SELECT count(*) FROM products`); n != 5 {
		t.Fatalf("products left = %d", n)
	}
}

func TestSearch_LengthRules(t *testing.T) {
	f, _, _ := searchWorld(t)
	for name, q := range map[string]string{"missing": "", "one char": "r", "spaces only": "%20%20%20", "too long": longText(61), "control char": "ri%00ce"} {
		t.Run(name, func(t *testing.T) {
			w := f.publicGet("/search?q=" + q)
			if w.Code != http.StatusBadRequest || decodeErr(f, w).Error.Fields["q"] == "" {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
	if w := f.publicGet("/search?q=" + longText(60)); w.Code != http.StatusOK {
		t.Fatalf("60 characters = %d", w.Code)
	}
}

func TestSearch_CheapestFirstAcrossDeliveringShops(t *testing.T) {
	f, _, _ := searchWorld(t)
	all := search(f, "q=rice")
	if got := fmt.Sprint(prices(all)); got != "[40000 48000 52000]" {
		t.Fatalf("without a pin = %s", got)
	}
	near := search(f, "q=rice&lat=6.9271&lng=79.8612") // the far shop (Kandy) does not deliver here
	if got := fmt.Sprint(prices(near)); got != "[48000 52000]" {
		t.Fatalf("with a pin = %s", got)
	}
	if p := near.Products.Data[0]; p.ShopSlug != "rice-house" || !p.Available || p.Unit != "PCS" {
		t.Fatalf("card = %+v", p)
	}
}

func TestSearch_ShopsWhoseNameMatch(t *testing.T) {
	f, _, _ := searchWorld(t)
	res := search(f, "q=rice")
	if len(res.Shops) != 1 || res.Shops[0].Slug != "rice-house" {
		t.Fatalf("matching shops = %+v", res.Shops)
	}
	if res := search(f, "q=tomato"); len(res.Shops) != 0 {
		t.Fatalf("shops = %+v", res.Shops)
	}
}

func TestSearch_InsideOneShop(t *testing.T) {
	f, _, _ := searchWorld(t)
	res := search(f, "q=rice&shop=near-shop")
	if len(res.Products.Data) != 1 || res.Products.Data[0].OnlinePriceCents != 52000 || len(res.Shops) != 0 {
		t.Fatalf("result = %+v", res)
	}
	f.a.SeedShop("draft-shop")
	for _, slug := range []string{"nope", "draft-shop"} {
		if w := f.publicGet("/search?q=rice&shop=" + slug); w.Code != http.StatusNotFound {
			t.Errorf("shop %q = %d", slug, w.Code)
		}
	}
}

func TestSearch_AvailableFirstAndPaged(t *testing.T) {
	f := newFixture(t)
	shop := f.activeShop("near-shop", 6.9271, 79.8612)
	for i := 0; i < 30; i++ {
		f.stock(shop, fmt.Sprintf("Rice %02d", i), int64(1000+i*100), nil, i != 0, nil) // the cheapest one is sold out
	}
	first := search(f, "q=rice")
	if len(first.Products.Data) != 24 || first.Products.NextCursor == nil || first.Products.Data[0].OnlinePriceCents != 1100 {
		t.Fatalf("first page: %d rows, cursor %v, first price %d", len(first.Products.Data), first.Products.NextCursor, first.Products.Data[0].OnlinePriceCents)
	}
	second := search(f, "q=rice&cursor="+*first.Products.NextCursor)
	if len(second.Products.Data) != 6 || second.Products.NextCursor != nil {
		t.Fatalf("second page: %d rows, cursor %v", len(second.Products.Data), second.Products.NextCursor)
	}
	last := second.Products.Data[5]
	if last.Available || last.OnlinePriceCents != 1000 {
		t.Fatalf("the sold out product should come last: %+v", last)
	}
	if w := f.publicGet("/search?q=rice&cursor=junk"); w.Code != http.StatusBadRequest {
		t.Fatalf("junk cursor = %d", w.Code)
	}
}

func TestSearch_HiddenProductsAreNotFound(t *testing.T) {
	f, near, _ := searchWorld(t)
	f.a.Exec(`UPDATE products SET status = 'HIDDEN' WHERE name = 'Tomato'`)
	if got := names(search(f, "q=tomato")); len(got) != 0 {
		t.Fatalf("hidden product found: %v", got)
	}
	_ = near
}

func prices(r catalog.SearchResult) []int64 {
	out := []int64{}
	for _, p := range r.Products.Data {
		out = append(out, p.OnlinePriceCents)
	}
	return out
}
