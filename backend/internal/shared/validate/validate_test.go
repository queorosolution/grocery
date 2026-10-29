package validate

import (
	"strings"
	"testing"

	"gp/internal/shared/apperr"
)

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	e := apperr.From(err)
	if e.Code != apperr.ValidationError {
		t.Fatalf("code = %s", e.Code)
	}
	return e.Fields
}

func TestStr(t *testing.T) {
	cases := []struct {
		name, in string
		min, max int
		want     string // reason, empty when valid
	}{
		{"ok", "Nimal", 1, 80, ""},
		{"trimmed before counting", "  ab  ", 3, 80, "too_short"},
		{"empty required", "   ", 1, 80, "required"},
		{"empty allowed", "", 0, 80, ""},
		{"too long counts characters not bytes", strings.Repeat("අ", 81), 1, 80, "too_long"},
		{"sinhala within limit", strings.Repeat("අ", 80), 1, 80, ""},
		{"control char", "a\x00b", 1, 80, "invalid"},
		{"newline not allowed in Str", "a\nb", 1, 80, "invalid"},
		{"bad utf8", "a\xffb", 1, 80, "invalid"},
	}
	for _, tc := range cases {
		err := New().Str("name", tc.in, tc.min, tc.max).Err()
		if tc.want == "" && err != nil {
			t.Errorf("%s: unexpected %v", tc.name, err)
		} else if tc.want != "" && fieldsOf(t, err)["name"] != tc.want {
			t.Errorf("%s: want %s, got %v", tc.name, tc.want, err)
		}
	}
}

func TestTextAllowsNewlinesButNotOtherControls(t *testing.T) {
	if err := New().Text("note", "line one\nline two\tok", 1, 300).Err(); err != nil {
		t.Fatal(err)
	}
	if err := New().Text("note", "bell\a", 1, 300).Err(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNormalizePhone(t *testing.T) {
	good := map[string]string{
		"0771234567":      "+94771234567",
		"+94771234567":    "+94771234567",
		"94771234567":     "+94771234567",
		"077 123 4567":    "+94771234567",
		"+94 77-123-4567": "+94771234567",
		"  0112345678 ":   "+94112345678",
	}
	for in, want := range good {
		if got, ok := NormalizePhone(in); !ok || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "0771234", "+447911123456", "07712345678", "abc", "+9407712345", "077-123-45a7", "00771234567"} {
		if got, ok := NormalizePhone(in); ok {
			t.Errorf("NormalizePhone(%q) accepted as %q", in, got)
		}
	}
}

func TestEmailAndUUID(t *testing.T) {
	if err := New().Email("e", "a@b.lk").UUID("id", "3f2a9c1e-5b7d-4e8a-9c21-7d4e5f6a8b90").Err(); err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"Name <a@b.lk>", "a@", "no-at", ""} {
		if New().Email("e", e).Err() == nil {
			t.Errorf("email %q accepted", e)
		}
	}
	for _, id := range []string{"", "123", "3f2a9c1e5b7d4e8a9c217d4e5f6a8b90", "{3f2a9c1e-5b7d-4e8a-9c21-7d4e5f6a8b90}"} {
		if New().UUID("id", id).Err() == nil {
			t.Errorf("uuid %q accepted", id)
		}
	}
}

func TestChainCollectsEveryFieldAndKeepsFirstReason(t *testing.T) {
	err := New().
		Str("name", "", 1, 10).
		Str("name", strings.Repeat("x", 99), 1, 10). // second failure on the same field is ignored
		Phone("phone", "123").
		Int("qty", 0, 1, 99).
		Check("status", false, "invalid").
		Err()
	got := fieldsOf(t, err)
	want := map[string]string{"name": "required", "phone": "invalid", "qty": "out_of_range", "status": "invalid"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}
}

func TestErrorsNeverEchoInput(t *testing.T) {
	secret := "<script>alert(1)</script>"
	err := New().Phone("phone", secret).Err()
	if strings.Contains(err.Error(), "script") || strings.Contains(apperr.From(err).Fields["phone"], "script") {
		t.Fatal("user input leaked into the error")
	}
}

func TestNoFailureMeansNilError(t *testing.T) {
	if New().Err() != nil {
		t.Fatal("empty validator must return nil, not a typed nil")
	}
}
