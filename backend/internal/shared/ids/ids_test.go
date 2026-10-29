package ids

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"gp/internal/shared/clock"
)

func TestEventIDsAreSortedByTime(t *testing.T) {
	defer clock.Freeze(time.UnixMilli(1_700_000_000_000))()
	a := Event()
	restore := clock.Freeze(time.UnixMilli(1_700_000_000_001))
	defer restore()
	b := Event()
	if !strings.HasPrefix(a, "evt_") || len(a) != 4+26 {
		t.Fatalf("bad id %q", a)
	}
	if a >= b {
		t.Fatalf("%q should sort before %q", a, b)
	}
}

func TestEventIDsStayUniqueInOneMillisecond(t *testing.T) {
	defer clock.Freeze(time.UnixMilli(1_700_000_000_000))()
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := Event()
		if seen[id] {
			t.Fatalf("duplicate %s", id)
		}
		seen[id] = true
	}
}

func TestOrderCodeShapeMatchesSchema(t *testing.T) {
	// orders.code CHECK in contracts/schema.sql: ^[A-Z0-9-]{6,16}$
	schema := regexp.MustCompile(`^[A-Z0-9-]{6,16}$`)
	shape := regexp.MustCompile(`^[A-Z]{2}-[0-9A-HJKMNP-TV-Z]{6}$`)
	for i := 0; i < 200; i++ {
		code := OrderCode("krishna-super")
		if !schema.MatchString(code) || !shape.MatchString(code) || !strings.HasPrefix(code, "KR-") {
			t.Fatalf("bad order code %q", code)
		}
	}
}

func TestShopPrefix(t *testing.T) {
	for in, want := range map[string]string{"krishna-super": "KR", "7-eleven": "EL", "a": "AX", "": "XX", "k s": "KS"} {
		if got := ShopPrefix(in); got != want {
			t.Errorf("ShopPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokenIsLongAndDifferent(t *testing.T) {
	a, b := Token(), Token()
	if len(a) != 43 || a == b {
		t.Fatalf("bad tokens %q %q", a, b)
	}
}

func TestRequestID(t *testing.T) {
	a := Request()
	if !strings.HasPrefix(a, "req_") || len(a) != 4+26 || a == Request() {
		t.Fatalf("bad request id %q", a)
	}
}
