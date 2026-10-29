package keyset

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

type key struct {
	At time.Time `json:"t"`
	ID uuid.UUID `json:"i"`
}

func TestRoundTrip(t *testing.T) {
	want := key{At: time.Date(2026, 10, 29, 3, 0, 0, 123456000, time.UTC), ID: uuid.New()}
	got, err := Decode(Encode(want), func(k key) uuid.UUID { return k.ID })
	if err != nil || got == nil || !got.At.Equal(want.At) || got.ID != want.ID {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
}

func TestDecode(t *testing.T) {
	id := func(k key) uuid.UUID { return k.ID }
	if k, err := Decode("", id); k != nil || err != nil {
		t.Errorf("empty = %v, %v; want the first page", k, err)
	}
	for _, bad := range []string{"%%%", "bm90LWpzb24", Encode(map[string]string{"x": "y"}), Encode(key{})} {
		if _, err := Decode(bad, id); err == nil {
			t.Errorf("Decode(%q) accepted", bad)
		}
	}
}
