package source

import (
	"strings"
	"testing"
	"time"
)

func TestReadSpec_SQL(t *testing.T) {
	have := []Column{{Name: "id"}, {Name: "name"}, {Name: "updated_at"}}
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	base := ReadSpec{Table: TableName{Schema: "public", Name: "products"}, Columns: []string{"id", "name"}, OrderBy: "id", Limit: 10}

	t.Run("plain read", func(t *testing.T) {
		q, args, err := base.sql(have)
		if err != nil || len(args) != 0 || !strings.HasSuffix(q, `ORDER BY "id" LIMIT 10`) || strings.Contains(q, "WHERE") {
			t.Fatalf("%q %v %v", q, args, err)
		}
	})
	t.Run("incremental read adds the change time and takes the cursor as an argument", func(t *testing.T) {
		s := base
		s.Change, s.Since = "updated_at", &since
		q, args, err := s.sql(have)
		if err != nil || len(args) != 1 || !strings.Contains(q, `"updated_at"::timestamptz >= $1`) || !strings.Contains(q, "to_char(") ||
			!strings.Contains(q, `ORDER BY "updated_at"::timestamptz, "id"`) || s.width() != 3 {
			t.Fatalf("%q %v %v", q, args, err)
		}
	})
	t.Run("ids are arguments, never part of the text", func(t *testing.T) {
		s := base
		s.Only = []string{"1'; DROP TABLE products; --"}
		q, args, err := s.sql(have)
		if err != nil || len(args) != 1 || strings.Contains(q, "DROP") || !strings.Contains(q, `"id"::text = ANY($1::text[])`) {
			t.Fatalf("%q %v %v", q, args, err)
		}
	})
	t.Run("both filters number their arguments in order", func(t *testing.T) {
		s := base
		s.Change, s.Since, s.Only = "updated_at", &since, []string{"a"}
		q, args, err := s.sql(have)
		if err != nil || len(args) != 2 || !strings.Contains(q, "$2::text[]") {
			t.Fatalf("%q %v %v", q, args, err)
		}
	})
	for name, s := range map[string]ReadSpec{
		"unknown change column": {Table: base.Table, Columns: base.Columns, OrderBy: "id", Limit: 1, Change: "nope"},
		"unsafe change column":  {Table: base.Table, Columns: base.Columns, OrderBy: "id", Limit: 1, Change: `x"; DROP`},
		"unknown order column":  {Table: base.Table, Columns: base.Columns, OrderBy: "nope", Limit: 1},
		"no limit":              {Table: base.Table, Columns: base.Columns, OrderBy: "id"},
	} {
		if _, _, err := s.sql(have); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
