package optional_test

import (
	"encoding/json"
	"testing"

	"gp/internal/shared/optional"
)

func TestFieldTellsOmittedFromNull(t *testing.T) {
	type body struct {
		A optional.Field[int64] `json:"a"`
	}
	tests := map[string]struct {
		json      string
		set, null bool
		want      *int64
	}{
		"omitted": {`{}`, false, false, nil},
		"null":    {`{"a":null}`, true, true, nil},
		"value":   {`{"a":7}`, true, false, ptr(7)},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var b body
			if err := json.Unmarshal([]byte(tc.json), &b); err != nil {
				t.Fatal(err)
			}
			if b.A.Set != tc.set || b.A.Null != tc.null {
				t.Fatalf("set=%v null=%v", b.A.Set, b.A.Null)
			}
			got := b.A.Ptr()
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("Ptr = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFieldRejectsWrongType(t *testing.T) {
	var b struct {
		A optional.Field[int64] `json:"a"`
	}
	if err := json.Unmarshal([]byte(`{"a":"x"}`), &b); err == nil {
		t.Fatal("string accepted for a number")
	}
}

func ptr(v int64) *int64 { return &v }
