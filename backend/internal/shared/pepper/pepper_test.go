package pepper

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"gp/internal/shared/secrets"
)

type fake map[string]string

func (f fake) Get(_ context.Context, name string) (string, error) {
	v, ok := f[name]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}

func TestHash(t *testing.T) {
	ctx := context.Background()
	s := fake{IP: "key-one", OTP: "key-two"}
	a, err := Hash(ctx, s, IP, "1.2.3.4")
	if err != nil || len(a) != 32 {
		t.Fatalf("hash: %x, %v", a, err)
	}
	if b, _ := Hash(ctx, s, IP, "1.2.3.4"); !bytes.Equal(a, b) {
		t.Error("same input must give the same hash")
	}
	if b, _ := Hash(ctx, s, OTP, "1.2.3.4"); bytes.Equal(a, b) {
		t.Error("another pepper must give another hash")
	}
	if b, _ := Hash(ctx, s, IP, "1.2.3.5"); bytes.Equal(a, b) {
		t.Error("another input must give another hash")
	}
	x, _ := Hash(ctx, s, IP, "ab", "c")
	if y, _ := Hash(ctx, s, IP, "a", "bc"); bytes.Equal(x, y) {
		t.Error("parts must not run together")
	}
	if _, err := Hash(ctx, fake{}, IP, "x"); !errors.Is(err, secrets.ErrNotFound) {
		t.Errorf("missing pepper: %v", err)
	}
}
