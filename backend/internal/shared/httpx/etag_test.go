package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func get(header string) (*httptest.ResponseRecorder, *http.Request) {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if header != "" {
		r.Header.Set("If-None-Match", header)
	}
	return httptest.NewRecorder(), r
}

func TestJSONWithETag(t *testing.T) {
	w, r := get("")
	JSONWithETag(w, r, http.StatusOK, map[string]int{"a": 1})
	tag := w.Header().Get("ETag")
	if w.Code != http.StatusOK || tag == "" || w.Body.String() != `{"a":1}` {
		t.Fatalf("first = %d %q tag %q", w.Code, w.Body, tag)
	}

	for name, header := range map[string]string{"same tag": tag, "weak": "W/" + tag, "in a list": `"x", ` + tag, "star": "*"} {
		w, r := get(header)
		JSONWithETag(w, r, http.StatusOK, map[string]int{"a": 1})
		if w.Code != http.StatusNotModified || w.Body.Len() != 0 || w.Header().Get("ETag") != tag {
			t.Errorf("%s = %d with %d bytes, want 304 and no body", name, w.Code, w.Body.Len())
		}
	}
	for name, header := range map[string]string{"another tag": `"other"`, "nothing": "", "junk": ","} {
		w, r := get(header)
		JSONWithETag(w, r, http.StatusOK, map[string]int{"a": 1})
		if w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Errorf("%s = %d, want 200 with the body", name, w.Code)
		}
	}

	// Another answer has another tag.
	w, r = get(tag)
	JSONWithETag(w, r, http.StatusOK, map[string]int{"a": 2})
	if w.Code != http.StatusOK || w.Header().Get("ETag") == tag {
		t.Errorf("changed body = %d, ETag %q; want 200 and a new tag", w.Code, w.Header().Get("ETag"))
	}
	// Only a plain 200 can be answered with 304.
	w, r = get(tag)
	JSONWithETag(w, r, http.StatusCreated, map[string]int{"a": 1})
	if w.Code != http.StatusCreated {
		t.Errorf("201 = %d", w.Code)
	}
	// A value that cannot be encoded is a clean 500, with no tag.
	w, r = get("")
	JSONWithETag(w, r, http.StatusOK, make(chan int))
	if w.Code != http.StatusInternalServerError || w.Header().Get("ETag") != "" {
		t.Errorf("unencodable = %d, ETag %q", w.Code, w.Header().Get("ETag"))
	}
}
