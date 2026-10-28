package catalog_test

import (
	"net/http"
	"net/url"
	"testing"

	"gp/internal/catalog"
	"gp/test/apptest"
)

func TestProduct_CreateUpdateAndSearch(t *testing.T) {
	f := newFixture(t)
	cat := f.category("rice")
	p := apptest.Decode[catalog.Product](t, f.admin.OK(http.MethodPost, "/admin/products",
		`{"name":"  Nadu Rice ","brand":"Araliya","barcode":"4791234567890","unit":"KG","weightG":1000,"categoryId":"`+cat.ID.String()+
			`","keywords":"red raw","imageUrl":"`+apptest.ImgBase+`/PRODUCT_IMAGE/a.webp"}`))
	if p.Name != "Nadu Rice" || p.Status != "ACTIVE" || p.Unit != "KG" || *p.Brand != "Araliya" {
		t.Fatalf("product = %+v", p)
	}
	f.product("Samba Rice", nil)
	f.product("Tea 100% pure_leaf", nil)

	got := apptest.Decode[catalog.Product](t, f.admin.OK(http.MethodPatch, "/admin/products/"+p.ID.String(),
		`{"brand":null,"status":"HIDDEN","weightG":null}`))
	if got.Brand != nil || got.WeightG != nil || got.Status != "HIDDEN" || got.Name != "Nadu Rice" || *got.Barcode != "4791234567890" {
		t.Fatalf("patch result = %+v", got)
	}

	search := func(q string) []catalog.Product {
		return apptest.Decode[catalog.Page[catalog.Product]](t, f.admin.OK(http.MethodGet, "/admin/products?q="+url.QueryEscape(q), "")).Data
	}
	for q, want := range map[string]int{"rice": 2, "RED": 1, "araliya": 0, "100%": 1, "pure_leaf": 1, "%": 1, "_": 1, "zzz": 0, "": 3} {
		if n := len(search(q)); n != want {
			t.Errorf("q=%q found %d, want %d", q, n, want)
		}
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE entity = 'product'`); n != 4 {
		t.Fatalf("audit rows = %d, want 4", n)
	}
}

func TestProduct_ListPages(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 53; i++ {
		f.a.Exec(`INSERT INTO products (name) VALUES ($1)`, "Item "+string(rune('A'+i/26))+string(rune('a'+i%26)))
	}
	first := apptest.Decode[catalog.Page[catalog.Product]](t, f.admin.OK(http.MethodGet, "/admin/products", ""))
	if len(first.Data) != 50 || first.NextCursor == "" {
		t.Fatalf("first page = %d rows, cursor %q", len(first.Data), first.NextCursor)
	}
	second := apptest.Decode[catalog.Page[catalog.Product]](t, f.admin.OK(http.MethodGet, "/admin/products?cursor="+first.NextCursor, ""))
	if len(second.Data) != 3 || second.NextCursor != "" || second.Data[0].Name <= first.Data[49].Name {
		t.Fatalf("second page = %d rows, cursor %q", len(second.Data), second.NextCursor)
	}
	f.admin.Expect(http.MethodGet, "/admin/products?cursor=not-a-cursor", "", http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestProduct_Rules(t *testing.T) {
	f := newFixture(t)
	f.product("Existing", nil)
	f.admin.OK(http.MethodPatch, "/admin/products/"+f.product("Other", nil).ID.String(), `{"barcode":"123456"}`)

	tests := map[string]struct{ body, field, reason string }{
		"javascript url (XSS-04)": {`{"name":"Rice","imageUrl":"javascript:alert(1)"}`, "imageUrl", "invalid"},
		"data url":                {`{"name":"Rice","imageUrl":"data:image/png;base64,AAAA"}`, "imageUrl", "invalid"},
		"other https host":        {`{"name":"Rice","imageUrl":"https://evil.example/a.png"}`, "imageUrl", "invalid"},
		"look-alike host":         {`{"name":"Rice","imageUrl":"https://img.example.test.evil.example/a.png"}`, "imageUrl", "invalid"},
		"user-info host":          {`{"name":"Rice","imageUrl":"https://img.example.test@evil.example/a.png"}`, "imageUrl", "invalid"},
		"plain http":              {`{"name":"Rice","imageUrl":"http://img.example.test/a.png"}`, "imageUrl", "invalid"},
		"path climbing":           {`{"name":"Rice","imageUrl":"https://img.example.test/../a.png"}`, "imageUrl", "invalid"},
		"short name":              {`{"name":"R"}`, "name", "too_short"},
		"no name":                 {`{}`, "name", "required"},
		"bad barcode":             {`{"name":"Rice","barcode":"12ab"}`, "barcode", "invalid"},
		"barcode taken":           {`{"name":"Rice","barcode":"123456"}`, "barcode", "taken"},
		"bad unit":                {`{"name":"Rice","unit":"BOX"}`, "unit", "invalid"},
		"bad status":              {`{"name":"Rice","status":"GONE"}`, "status", "invalid"},
		"weight out of range":     {`{"name":"Rice","weightG":0}`, "weightG", "out_of_range"},
		"unknown category":        {`{"name":"Rice","categoryId":"` + missing + `"}`, "categoryId", "unknown"},
		"long keywords":           {`{"name":"Rice","keywords":"` + longText(301) + `"}`, "keywords", "too_long"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := f.admin.Expect(http.MethodPost, "/admin/products", tc.body, http.StatusBadRequest, "VALIDATION_ERROR")
			if e.Error.Fields[tc.field] != tc.reason {
				t.Fatalf("fields = %v, want %s=%s", e.Error.Fields, tc.field, tc.reason)
			}
		})
	}
	if n := f.a.Count(`SELECT count(*) FROM products WHERE image_url IS NOT NULL`); n != 0 {
		t.Fatalf("%d products kept a rejected image url", n)
	}
	f.admin.Expect(http.MethodPatch, "/admin/products/"+missing, `{"name":"Rice"}`, http.StatusNotFound, "NOT_FOUND")
}

func TestProduct_SearchTextFollowsTheName(t *testing.T) {
	f := newFixture(t)
	p := f.product("Old Name", nil)
	f.admin.OK(http.MethodPatch, "/admin/products/"+p.ID.String(), `{"name":"Fresh Name"}`)
	if n := f.a.Count(`SELECT count(*) FROM products WHERE search_doc LIKE '%fresh%'`); n != 1 {
		t.Fatalf("search_doc not updated: %d", n)
	}
}

func longText(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'k'
	}
	return string(b)
}
