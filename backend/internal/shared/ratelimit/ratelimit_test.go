package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/ratelimit"
	"gp/test/testdb"
)

func TestHit(t *testing.T) {
	env := testdb.New(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC)
	defer clock.Freeze(now)()

	hit := func(key string) error {
		return ratelimit.Hit(ctx, env.DB, key, 2, time.Minute)
	}
	for i := 0; i < 2; i++ {
		if err := hit("a"); err != nil {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	if err := hit("a"); !apperr.Is(err, apperr.RateLimited) {
		t.Fatalf("third attempt = %v, want RATE_LIMITED", err)
	}
	if err := hit("b"); err != nil {
		t.Fatalf("other key must not be limited: %v", err)
	}
	clock.Freeze(now.Add(time.Minute))
	if err := hit("a"); err != nil {
		t.Fatalf("new window must reset: %v", err)
	}
}
