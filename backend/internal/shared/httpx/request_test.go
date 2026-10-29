package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gp/internal/shared/apperr"
)

type input struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

func post(body, contentType string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	return r
}

func TestDecode(t *testing.T) {
	cases := []struct {
		name, body, ct string
		code           apperr.Code
		field, reason  string
	}{
		{"valid", `{"name":"rice","qty":2}`, "application/json", "", "", ""},
		{"valid with charset", `{"name":"rice"}`, "application/json; charset=utf-8", "", "", ""},
		{"unknown field", `{"name":"a","price":1}`, "application/json", apperr.ValidationError, "body", "unknown_field"},
		{"wrong type names our field", `{"qty":"two"}`, "application/json", apperr.ValidationError, "qty", "invalid_type"},
		{"syntax error", `{"name":`, "application/json", apperr.ValidationError, "body", "invalid"},
		{"empty body", ``, "application/json", apperr.ValidationError, "body", "required"},
		{"trailing data", `{"name":"a"} {"name":"b"}`, "application/json", apperr.ValidationError, "body", "invalid"},
		{"wrong content type", `{"name":"a"}`, "text/plain", apperr.ValidationError, "contentType", "invalid"},
		{"form content type", `name=a`, "application/x-www-form-urlencoded", apperr.ValidationError, "contentType", "invalid"},
		{"too big", `{"name":"` + strings.Repeat("a", MaxBody) + `"}`, "application/json", apperr.PayloadTooLarge, "", ""},
	}
	for _, tc := range cases {
		var in input
		err := Decode(post(tc.body, tc.ct), &in)
		if tc.code == "" {
			if err != nil {
				t.Errorf("%s: unexpected %v", tc.name, err)
			}
			continue
		}
		e := apperr.From(err)
		if e.Code != tc.code || (tc.field != "" && e.Fields[tc.field] != tc.reason) {
			t.Errorf("%s: got %s %v, want %s %s=%s", tc.name, e.Code, e.Fields, tc.code, tc.field, tc.reason)
		}
	}
}

func TestDecodeFillsTheStruct(t *testing.T) {
	var in input
	if err := Decode(post(`{"name":"rice","qty":2}`, "application/json"), &in); err != nil || in.Name != "rice" || in.Qty != 2 {
		t.Fatalf("got %+v, %v", in, err)
	}
}

func TestDecodeNeverEchoesClientInput(t *testing.T) {
	err := Decode(post(`{"<script>alert(1)</script>":1}`, "application/json"), &input{})
	if err == nil || strings.Contains(err.Error(), "script") || strings.Contains(apperr.From(err).Fields["body"], "script") {
		t.Fatalf("client input leaked: %v", err)
	}
}

func TestPageLimit(t *testing.T) {
	if n, err := PageLimit("", 20); n != 20 || err != nil {
		t.Errorf("empty = %d, %v", n, err)
	}
	if n, err := PageLimit("7", 20); n != 7 || err != nil {
		t.Errorf("7 = %d, %v", n, err)
	}
	for _, bad := range []string{"x", "1.5", "-", "9999999999999999999999"} {
		if _, err := PageLimit(bad, 20); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
