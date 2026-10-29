package connector

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Counts is what one run did, as shown in the run list.
type Counts struct {
	Read      int `json:"rowsRead"`
	Changed   int `json:"rowsChanged"`
	Unmatched int `json:"rowsUnmatched"`
	Held      int `json:"rowsHeld"`
}

func (c *Counts) add(o Counts) {
	c.Read += o.Read
	c.Changed += o.Changed
	c.Unmatched += o.Unmatched
	c.Held += o.Held
}

// cursorOf is where the last good incremental run stopped.
func cursorOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (*time.Time, error) {
	var at *time.Time
	err := tx.QueryRow(ctx, `SELECT last_cursor FROM connectors WHERE id = $1`, id).Scan(&at)
	return at, err
}

// failStale ends runs that were left RUNNING (the function was cut off) so they
// stop blocking new ones.
func failStale(ctx context.Context, tx *db.Tx, id uuid.UUID, cutoff time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE connector_runs SET status = $3, error = 'timed_out', finished_at = now()
		WHERE connector_id = $1 AND status = $4 AND started_at < $2`,
		id, cutoff, enum.RunStatusFailed, enum.RunStatusRunning)
	return err
}

func hasRunning(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error) {
	var busy bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM connector_runs WHERE connector_id = $1 AND status = $2)`,
		id, enum.RunStatusRunning).Scan(&busy)
	return busy, err
}

// insertRun adds a run row. A SKIPPED row is already finished.
func insertRun(ctx context.Context, tx *db.Tx, id uuid.UUID, trigger enum.RunTrigger, status enum.RunStatus) (uuid.UUID, error) {
	var runID uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO connector_runs (connector_id, trigger, status, finished_at)
		VALUES ($1, $2, $3, CASE WHEN $3::text = 'RUNNING' THEN NULL ELSE now() END) RETURNING id`,
		id, trigger, status).Scan(&runID)
	return runID, err
}

func touchLastRun(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE connectors SET last_run_at = now() WHERE id = $1`, id)
	return err
}

func finishRun(ctx context.Context, tx *db.Tx, runID uuid.UUID, status enum.RunStatus, c Counts, errText string) error {
	_, err := tx.Exec(ctx, `
		UPDATE connector_runs SET status = $2, rows_read = $3, rows_changed = $4, rows_unmatched = $5, rows_held = $6,
		       error = NULLIF($7, ''), finished_at = now()
		WHERE id = $1`, runID, status, c.Read, c.Changed, c.Unmatched, c.Held, errText)
	return err
}

// markSuccess clears the failure count and moves the cursor forward.
func markSuccess(ctx context.Context, tx *db.Tx, id uuid.UUID, cursor *time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE connectors SET last_success_at = now(), consecutive_failures = 0,
		       status = CASE WHEN status = 'FAILING' THEN 'ACTIVE' ELSE status END,
		       last_cursor = COALESCE($2, last_cursor)
		WHERE id = $1`, id, cursor)
	return err
}

// markFailure counts a failed run. The third one in a row makes an ACTIVE
// connector FAILING; the count it returns tells the caller to raise the alert.
func markFailure(ctx context.Context, tx *db.Tx, id uuid.UUID) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		UPDATE connectors SET consecutive_failures = consecutive_failures + 1,
		       status = CASE WHEN consecutive_failures + 1 >= $2 AND status = 'ACTIVE' THEN 'FAILING' ELSE status END
		WHERE id = $1 RETURNING consecutive_failures`, id, failuresBeforeAlert).Scan(&n)
	return n, err
}

// Run is one row of the run list.
type Run struct {
	ID         uuid.UUID       `json:"id"`
	Trigger    enum.RunTrigger `json:"trigger"`
	Status     enum.RunStatus  `json:"status"`
	Counts                     // flattened: rowsRead, rowsChanged, ...
	Error      *string         `json:"error"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
}

func recentRuns(ctx context.Context, tx *db.Tx, id uuid.UUID, limit int) ([]Run, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, trigger, status, rows_read, rows_changed, rows_unmatched, rows_held, error, started_at, finished_at
		FROM connector_runs WHERE connector_id = $1 ORDER BY started_at DESC, id LIMIT $2`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.Trigger, &r.Status, &r.Read, &r.Changed, &r.Unmatched, &r.Held, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
