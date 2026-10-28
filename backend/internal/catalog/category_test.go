package catalog_test

import (
	"net/http"
	"testing"

	"gp/internal/catalog"
	"gp/test/apptest"
)

func TestCategory_CreateListUpdate(t *testing.T) {
	f := newFixture(t)
	rice := f.category("rice")
	if rice.DefaultWeightG != 500 || rice.ParentID != nil {
		t.Fatalf("defaults wrong: %+v", rice)
	}
	child := apptest.Decode[catalog.Category](t, f.admin.OK(http.MethodPost, "/admin/categories",
		`{"slug":"basmati","name":"Basmati","parentId":"`+rice.ID.String()+`","sort":2,"defaultWeightG":1000}`))
	if child.ParentID == nil || *child.ParentID != rice.ID {
		t.Fatalf("parent not kept: %+v", child)
	}
	got := apptest.Decode[catalog.Category](t, f.admin.OK(http.MethodPatch, "/admin/categories/"+child.ID.String(), `{"name":"Basmati rice"}`))
	if got.Name != "Basmati rice" || got.Slug != "basmati" || got.Sort != 2 {
		t.Fatalf("patch changed too much or too little: %+v", got)
	}
	list := apptest.Decode[struct{ Data []catalog.Category }](t, f.admin.OK(http.MethodGet, "/admin/categories", "")).Data
	if len(list) != 2 {
		t.Fatalf("list = %d categories", len(list))
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE entity = 'category'`); n != 3 {
		t.Fatalf("audit rows = %d, want 3", n)
	}
}

// missing is an id no test creates.
const missing = "00000000-0000-0000-0000-000000000001"

func TestCategory_Rules(t *testing.T) {
	f := newFixture(t)
	rice := f.category("rice")
	child := apptest.Decode[catalog.Category](t, f.admin.OK(http.MethodPost, "/admin/categories",
		`{"slug":"basmati","name":"B","parentId":"`+rice.ID.String()+`"}`))
	other := f.category("pulses")

	tests := map[string]struct {
		method, path, body string
		field, reason      string
	}{
		"slug taken":         {http.MethodPost, "/admin/categories", `{"slug":"rice","name":"Again"}`, "slug", "taken"},
		"bad slug":           {http.MethodPost, "/admin/categories", `{"slug":"Rice Cakes","name":"x"}`, "slug", "invalid"},
		"no name":            {http.MethodPost, "/admin/categories", `{"slug":"tea"}`, "name", "required"},
		"weight too big":     {http.MethodPost, "/admin/categories", `{"slug":"tea","name":"Tea","defaultWeightG":100001}`, "defaultWeightG", "out_of_range"},
		"three levels":       {http.MethodPost, "/admin/categories", `{"slug":"tea","name":"Tea","parentId":"` + child.ID.String() + `"}`, "parentId", "invalid"},
		"unknown parent":     {http.MethodPost, "/admin/categories", `{"slug":"tea","name":"Tea","parentId":"` + missing + `"}`, "parentId", "invalid"},
		"own parent":         {http.MethodPatch, "/admin/categories/" + other.ID.String(), `{"parentId":"` + other.ID.String() + `"}`, "parentId", "invalid"},
		"parent with kids":   {http.MethodPatch, "/admin/categories/" + rice.ID.String(), `{"parentId":"` + other.ID.String() + `"}`, "parentId", "has_children"},
		"slug taken (patch)": {http.MethodPatch, "/admin/categories/" + other.ID.String(), `{"slug":"rice"}`, "slug", "taken"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := f.admin.Expect(tc.method, tc.path, tc.body, http.StatusBadRequest, "VALIDATION_ERROR")
			if e.Error.Fields[tc.field] != tc.reason {
				t.Fatalf("fields = %v, want %s=%s", e.Error.Fields, tc.field, tc.reason)
			}
		})
	}
	f.admin.Expect(http.MethodPatch, "/admin/categories/"+missing, `{"name":"x"}`, http.StatusNotFound, "NOT_FOUND")
}

func TestCategory_ParentCanBeCleared(t *testing.T) {
	f := newFixture(t)
	rice := f.category("rice")
	child := apptest.Decode[catalog.Category](t, f.admin.OK(http.MethodPost, "/admin/categories",
		`{"slug":"basmati","name":"B","parentId":"`+rice.ID.String()+`"}`))
	got := apptest.Decode[catalog.Category](t, f.admin.OK(http.MethodPatch, "/admin/categories/"+child.ID.String(), `{"parentId":null}`))
	if got.ParentID != nil {
		t.Fatalf("parent = %v, want none", got.ParentID)
	}
}
