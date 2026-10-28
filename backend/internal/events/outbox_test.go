package events_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/test/testdb"
)

func TestAdd(t *testing.T) {
	env := testdb.New(t)
	ctx := context.Background()
	var id string
	err := env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		id, err = events.Add(ctx, tx, events.Event{Type: enum.EventTypeSiteRebuild, EntityID: "shop-1", Payload: map[string]string{"reason": "test"}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var typ, payload string
	var shop *uuid.UUID
	var sent bool
	if err := env.Owner.QueryRow(ctx, `SELECT type, shop_id, payload::text, sent_at IS NOT NULL FROM outbox WHERE id = $1`, id).Scan(&typ, &shop, &payload, &sent); err != nil {
		t.Fatal(err)
	}
	if typ != "site.rebuild" || shop != nil || sent || payload != `{"reason": "test"}` {
		t.Fatalf("row = %q %v %q sent=%v", typ, shop, payload, sent)
	}
}

func TestAddRollsBackWithTheTransaction(t *testing.T) {
	env := testdb.New(t)
	ctx := context.Background()
	_ = env.DB.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if _, err := events.Add(ctx, tx, events.Event{Type: enum.EventTypeSiteRebuild}); err != nil {
			t.Fatal(err)
		}
		return context.Canceled // the change failed, so the event must vanish
	})
	var n int
	if err := env.Owner.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("outbox rows = %d, err %v", n, err)
	}
}

func TestAddRejectsUnknownType(t *testing.T) {
	env := testdb.New(t)
	err := env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		_, err := events.Add(ctx, tx, events.Event{Type: "made.up"})
		return err
	})
	if err == nil {
		t.Fatal("unknown event type accepted")
	}
}
