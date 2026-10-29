package connector

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"time"

	"gp/internal/connector/source"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
)

// sync reads the source and saves the rows, 200 at a time. The read is over
// (and the connection closed) before the first batch is saved, so no database
// transaction waits on the network.
//
// With a change column the read is incremental and the returned cursor is the
// newest change time seen. Without one every row is read, unchanged rows are
// skipped by their hash, and (when the whole table was read) managed products
// that are no longer in the source become unavailable (FR-SYNC-07, FR-SYNC-11).
func (s *Service) sync(ctx context.Context, c Connector, job Job) (Counts, *time.Time, error) {
	var cursorIn *time.Time
	if c.changeColumn() != "" && len(job.ExternalIDs) == 0 {
		if err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
			cursorIn, err = cursorOf(ctx, tx, c.ID)
			return err
		}); err != nil {
			return Counts{}, nil, err
		}
	}

	var rows []source.Row
	var slot map[string]int
	err := s.with(ctx, db.System(), c.ID, "run", func(ctx context.Context, fresh Connector, conn source.Conn) error {
		spec, sl, err := readSpec(fresh, maxRows+1)
		if err != nil {
			return err
		}
		slot = sl
		spec.Change, spec.Since, spec.Only = fresh.changeColumn(), cursorIn, job.ExternalIDs
		return conn.Read(ctx, spec, func(r source.Row) error {
			rows = append(rows, r)
			return nil
		})
	})
	if err != nil {
		return Counts{}, nil, err
	}
	truncated := len(rows) > maxRows
	if truncated {
		rows = rows[:maxRows]
		logx.From(ctx).Warn("connector.run_truncated", "connector", c.ID, "limit", maxRows)
	}

	full := c.changeColumn() == "" && len(job.ExternalIDs) == 0
	var total Counts
	seen := make([]string, 0, len(rows))
	for start := 0; start < len(rows); start += batchSize {
		batch := rows[start:min(start+batchSize, len(rows))]
		var got Counts
		var ids []string
		err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
			got, ids, err = s.applyBatch(ctx, tx, c, batch, slot, full)
			return err
		})
		if err != nil {
			return total, nil, err
		}
		total.add(got)
		seen = append(seen, ids...)
	}

	if full && !truncated {
		var gone int
		err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
			gone, err = hideMissing(ctx, tx, c.ShopID, seen)
			return err
		})
		if err != nil {
			return total, nil, err
		}
		total.Changed += gone
	}
	if len(job.ExternalIDs) > 0 {
		return total, nil, nil // a run for chosen ids moves no cursor
	}
	return total, lastChange(rows, c), nil
}

// changeColumn is the column whose time moves when a row changes ("" = none).
func (c Connector) changeColumn() string {
	if c.ChangeColumn == nil {
		return ""
	}
	return *c.ChangeColumn
}

// lastChange is the change time of the newest row read: the extra last value of
// each row, which comes oldest first.
func lastChange(rows []source.Row, c Connector) *time.Time {
	if c.changeColumn() == "" || len(rows) == 0 {
		return nil
	}
	last := rows[len(rows)-1]
	if v := last[len(last)-1]; v != nil {
		if t, err := time.Parse(time.RFC3339Nano, *v); err == nil {
			return &t
		}
	}
	return nil
}

// hashRow is the fingerprint of a raw row. Each value is length-prefixed, so
// ("ab","c") and ("a","bc") differ; a NULL differs from an empty text.
func hashRow(row source.Row) []byte {
	h := sha256.New()
	var n [8]byte
	for _, v := range row {
		if v == nil {
			binary.BigEndian.PutUint64(n[:], ^uint64(0))
			h.Write(n[:])
			continue
		}
		binary.BigEndian.PutUint64(n[:], uint64(len(*v)))
		h.Write(n[:])
		h.Write([]byte(*v))
	}
	return h.Sum(nil)
}
