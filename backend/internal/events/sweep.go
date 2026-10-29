package events

import (
	"context"
	"time"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
)

const (
	// sweepAge leaves a fresh event to its own after-commit publish.
	sweepAge = 2 * time.Minute
	// sweepRound and sweepRounds bound one run: 5 x 100 events, then the next run continues.
	sweepRound  = 100
	sweepRounds = 5
	// keepProcessed is how long duplicate-protection rows are kept (docs/04 section 5).
	keepProcessed = 7 * 24 * time.Hour
)

// SweepResult says what one sweep did.
type SweepResult struct {
	Found  int `json:"found"`
	Sent   int `json:"sent"`
	Pruned int `json:"pruned"`
}

// Sweep sends events whose publish was missed (queue down, Lambda ended early):
// unsent rows older than two minutes, oldest first. It also removes old
// processed_events rows. It is safe to run twice at once: a duplicate send is
// absorbed by the consumers' idempotency.
func (p *Publisher) Sweep(ctx context.Context) (SweepResult, error) {
	var res SweepResult
	cutoff := clock.Now().Add(-sweepAge)
	for i := 0; i < sweepRounds; i++ {
		var msgs []Message
		err := p.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
			msgs, err = unsent(ctx, tx, cutoff, sweepRound)
			return err
		})
		if err != nil {
			return res, err
		}
		res.Found += len(msgs)
		if len(msgs) == 0 {
			break
		}
		sent, err := p.publish(ctx, msgs)
		res.Sent += sent
		if err != nil {
			// Report and stop: the next run retries from the oldest row.
			logx.From(ctx).Warn("events.sweep_publish_failed", "err", err)
			return res, err
		}
		if len(msgs) < sweepRound {
			break
		}
	}
	err := p.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM processed_events WHERE at < $1`, clock.Now().Add(-keepProcessed))
		res.Pruned = int(tag.RowsAffected())
		return err
	})
	return res, err
}

func unsent(ctx context.Context, tx *db.Tx, cutoff time.Time, limit int) ([]Message, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, type, shop_id, coalesce(entity_id, ''), payload, created_at FROM outbox
		WHERE sent_at IS NULL AND created_at < $1 ORDER BY created_at, id LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Type, &m.ShopID, &m.EntityID, &m.Data, &m.At); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
