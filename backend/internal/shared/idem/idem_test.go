package idem

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKey(t *testing.T) {
	cases := []struct {
		key  string
		want string // "" means accepted, else the field reason
	}{
		{"0123456789abcdef", ""},
		{strings.Repeat("a", 64), ""},
		{"abc_DEF-0123456789", ""},
		{"", "required"},
		{"short", "invalid"},
		{strings.Repeat("a", 65), "invalid"},
		{"has a space 0123456", "invalid"},
		{"semi;colon0123456789", "invalid"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/v1/bills", nil)
		if c.key != "" {
			r.Header.Set("Idempotency-Key", c.key)
		}
		got, err := Key(r)
		switch {
		case c.want == "" && (err != nil || got != c.key):
			t.Errorf("Key(%q) = %q, %v; want it accepted", c.key, got, err)
		case c.want != "" && err == nil:
			t.Errorf("Key(%q) accepted; want %s", c.key, c.want)
		case c.want != "" && !strings.Contains(err.Error(), "VALIDATION"):
			t.Errorf("Key(%q) = %v", c.key, err)
		}
	}
}

func TestHash(t *testing.T) {
	type body struct {
		A int
		B string
	}
	a, _ := Hash(body{1, "x"})
	b, _ := Hash(body{1, "x"})
	c, _ := Hash(body{2, "x"})
	if string(a) != string(b) || string(a) == string(c) || len(a) != 32 {
		t.Error("the same body must hash the same and a changed body differently")
	}
}
