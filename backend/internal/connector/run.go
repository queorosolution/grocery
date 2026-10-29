package connector

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/logx"
)

const (
	// runStaleAfter is how long a RUNNING row blocks the next run. The sync
	// function stops after 60 s, so an older row means it was cut off.
	runStaleAfter = 2 * time.Minute
	// maxRows is the most one run reads (FR-SYNC-13); batchSize rows are saved per transaction.
	maxRows   = 20000
	batchSize = 200
	// failuresBeforeAlert failed runs in a row make the connector FAILING and raise connector.failed.
	failuresBeforeAlert = 3
	// finishTimeout lets a run write its result even when its own time is up.
	finishTimeout = 10 * time.Second
)

// Fixed texts for connector_runs.error. They say which step failed and nothing
// about the shop's database, so the run list is safe to show.
const (
	errSource = "source_failed" // could not connect or read
	errApply  = "apply_failed"  // could not save into our database
)

// RunResult is what Run did, for the caller and the log.
type RunResult struct {
	RunID  uuid.UUID      `json:"runId"`
	Status enum.RunStatus `json:"status"`
	Counts
}

// Run is one sync of a shop's database into its catalog (docs/07 section 11).
// It never overlaps another run of the same connector. A manual run that finds
// one in progress answers CONNECTOR_BUSY; a scheduled or webhook run is
// recorded as SKIPPED. A scheduled run outside open hours does nothing at all
// and writes no row, so a closed night does not fill the run list.
func (s *Service) Run(ctx context.Context, job Job) (RunResult, error) {
	c, runID, skipped, err := s.begin(ctx, job)
	if err != nil || runID == uuid.Nil {
		return skipped, err
	}
	counts, cursor, err := s.sync(ctx, c, job)
	return s.finish(ctx, c, runID, counts, cursor, err)
}

// begin decides whether the run goes ahead and, if so, records it as RUNNING.
// When it does not, the result says SKIPPED and the run id is nil.
func (s *Service) begin(ctx context.Context, job Job) (c Connector, runID uuid.UUID, skipped RunResult, err error) {
	skipped = RunResult{Status: enum.RunStatusSkipped}
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		var found bool
		var err error
		if c, found, err = byID(ctx, tx, job.ConnectorID, true); err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if c.SourceTable == nil || c.Columns["externalId"] == "" {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"mapping": "not_set"}}
		}
		manual := job.Trigger == enum.RunTriggerManual
		if !manual && c.Status != enum.ConnectorStatusActive && c.Status != enum.ConnectorStatusFailing {
			return nil // paused or not live yet: a late schedule or a webhook does nothing
		}
		if err := failStale(ctx, tx, c.ID, clock.Now().Add(-runStaleAfter)); err != nil {
			return err
		}
		busy, err := hasRunning(ctx, tx, c.ID)
		if err != nil {
			return err
		}
		if busy {
			if skipped.RunID, err = insertRun(ctx, tx, c.ID, job.Trigger, enum.RunStatusSkipped); err != nil {
				return err
			}
			if manual {
				return apperr.New(apperr.ConnectorBusy)
			}
			return nil
		}
		if job.Trigger == enum.RunTriggerSchedule {
			if s.rt.Hours == nil {
				return errors.New("connector: no opening hours source")
			}
			open, err := s.rt.Hours.OpenAt(ctx, tx, c.ShopID, clock.Now())
			if err != nil || !open {
				return err
			}
		}
		if runID, err = insertRun(ctx, tx, c.ID, job.Trigger, enum.RunStatusRunning); err != nil {
			return err
		}
		return touchLastRun(ctx, tx, c.ID)
	})
	var app *apperr.Error
	if errors.As(err, &app) && app.Code == apperr.ConnectorBusy {
		// The SKIPPED row is rolled back with the transaction; the caller only needs the error.
		return c, uuid.Nil, skipped, err
	}
	return c, runID, skipped, err
}

// finish writes the result of the run: counts, the cursor, and the failure
// count with its alert. It uses its own time limit so a run that ran out of
// time can still say so.
func (s *Service) finish(ctx context.Context, c Connector, runID uuid.UUID, counts Counts, cursor *time.Time, runErr error) (RunResult, error) {
	res := RunResult{RunID: runID, Status: enum.RunStatusSuccess, Counts: counts}
	errText := ""
	if runErr != nil {
		res.Status, errText = enum.RunStatusFailed, errApply
		var app *apperr.Error
		if errors.As(runErr, &app) && app.Code == apperr.ConnectorTestFailed {
			errText = errSource
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if err := finishRun(ctx, tx, runID, res.Status, counts, errText); err != nil {
			return err
		}
		if runErr == nil {
			return markSuccess(ctx, tx, c.ID, cursor)
		}
		n, err := markFailure(ctx, tx, c.ID)
		if err != nil || n != failuresBeforeAlert {
			return err
		}
		_, err = events.Add(ctx, tx, events.Event{Type: enum.EventTypeConnectorFailed, ShopID: c.ShopID, EntityID: c.ID.String(),
			Payload: map[string]any{"connectorId": c.ID, "failures": n, "error": errText}})
		return err
	})
	if err != nil {
		return res, err
	}
	logx.From(ctx).Info("connector.run", "connector", c.ID, "status", res.Status, "read", counts.Read,
		"changed", counts.Changed, "unmatched", counts.Unmatched, "held", counts.Held)
	return res, runErr
}
