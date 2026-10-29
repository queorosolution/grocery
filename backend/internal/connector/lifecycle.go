package connector

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// runsShown is how many runs the run list returns (docs/06 section 10).
const runsShown = 50

// Activate starts the connector: from now on it syncs on its schedule and on
// triggers. It needs a saved mapping. Activating a live connector is fine and
// puts its schedule right again; a FAILING one starts with a clean count.
func (s *Service) Activate(ctx context.Context, scope db.Scope, id uuid.UUID) (Connector, error) {
	out, err := s.setStatus(ctx, scope, id, "connector.activate", enum.ConnectorStatusActive,
		func(c Connector) error {
			if c.SourceTable == nil || c.Columns["externalId"] == "" {
				return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"mapping": "not_set"}}
			}
			return nil
		})
	if err != nil {
		return out, err
	}
	return out, s.keepSchedule(ctx, out)
}

// Pause stops the schedule and ignores triggers. Nothing else changes, so
// activating again continues where it stopped.
func (s *Service) Pause(ctx context.Context, scope db.Scope, id uuid.UUID) (Connector, error) {
	out, err := s.setStatus(ctx, scope, id, "connector.pause", enum.ConnectorStatusPaused,
		func(c Connector) error {
			if c.Status == enum.ConnectorStatusDraft {
				return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"status": "not_active"}}
			}
			return nil
		})
	if err != nil {
		return out, err
	}
	return out, s.keepSchedule(ctx, out)
}

// setStatus moves a connector to a status in one locked transaction and audits it.
func (s *Service) setStatus(ctx context.Context, scope db.Scope, id uuid.UUID, action string, to enum.ConnectorStatus,
	check func(Connector) error) (Connector, error) {
	var out Connector
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := byID(ctx, tx, id, true)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if err := check(before); err != nil {
			return err
		}
		if out, err = scanConnector(tx.QueryRow(ctx,
			`UPDATE connectors SET status = $2, consecutive_failures = CASE WHEN $2::text = 'ACTIVE' THEN 0 ELSE consecutive_failures END
			 WHERE id = $1 RETURNING `+connectorColumns, id, to)); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: action, Entity: "connector", EntityID: id.String(), ShopID: before.ShopID,
			Before: map[string]any{"status": before.Status}, After: map[string]any{"status": out.Status},
		})
	})
	return out, err
}

// keepSchedule makes the recurring schedule match the connector: one for a live
// connector, none otherwise. It runs after the commit, so a failure here leaves
// the state saved; repeating the same call sets the schedule right.
func (s *Service) keepSchedule(ctx context.Context, c Connector) error {
	if s.rt.Schedules == nil {
		return nil
	}
	var err error
	if c.Status == enum.ConnectorStatusActive || c.Status == enum.ConnectorStatusFailing {
		err = s.rt.Schedules.Upsert(ctx, c.ID, c.IntervalMin)
	} else {
		err = s.rt.Schedules.Delete(ctx, c.ID)
	}
	if err != nil {
		return apperr.New(apperr.Internal).Wrap(err)
	}
	return nil
}

// SyncNow starts a run for the admin's "Sync now" button and does not wait for
// it. A run already in progress is CONNECTOR_BUSY. Without an invoker (local
// runs, tests) the run is done here before returning.
func (s *Service) SyncNow(ctx context.Context, scope db.Scope, id uuid.UUID) error {
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		c, found, err := byID(ctx, tx, id, false)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if c.SourceTable == nil || c.Columns["externalId"] == "" {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"mapping": "not_set"}}
		}
		var busy bool
		err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM connector_runs WHERE connector_id = $1 AND status = $2 AND started_at > $3)`,
			id, enum.RunStatusRunning, clock.Now().Add(-runStaleAfter)).Scan(&busy)
		if err == nil && busy {
			return apperr.New(apperr.ConnectorBusy)
		}
		return err
	})
	if err != nil {
		return err
	}
	return s.start(ctx, Job{ConnectorID: id, Trigger: enum.RunTriggerManual})
}

// start hands a job to the sync function, or runs it here when there is none.
func (s *Service) start(ctx context.Context, job Job) error {
	if s.rt.Invoker != nil {
		if err := s.rt.Invoker.Invoke(ctx, job); err != nil {
			return apperr.New(apperr.Internal).Wrap(err)
		}
		return nil
	}
	_, err := s.Run(ctx, job)
	return err
}

// Runs lists the last runs of a connector, newest first.
func (s *Service) Runs(ctx context.Context, scope db.Scope, id uuid.UUID) ([]Run, error) {
	var out []Run
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if _, found, err := byID(ctx, tx, id, false); err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		var err error
		out, err = recentRuns(ctx, tx, id, runsShown)
		return err
	})
	return out, err
}
