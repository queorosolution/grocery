// Package syncjob is the sync Lambda: it receives a connector job from the
// schedule, from "Sync now" or from the trigger URL, and runs it. cmd/sync only
// wires this package to the Lambda runtime.
package syncjob

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/catalog"
	"gp/internal/connector"
	"gp/internal/connector/source"
	"gp/internal/pricing"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
	"gp/internal/shop"
)

// Job runs one connector job and returns the error of the run, so the invoke
// is marked failed and retried by Lambda only for faults it can fix: a failed
// source is recorded in the run list and counted, not retried.
type Job struct{ svc *connector.Service }

// New builds the job around a connector service.
func New(svc *connector.Service) *Job { return &Job{svc: svc} }

// Wire builds the sync function's dependencies and returns a function that
// closes what it opened.
func Wire(ctx context.Context, cfg *config.Config, sec secrets.Store) (*Job, func(), error) {
	database, err := db.Connect(ctx, cfg, sec)
	if err != nil {
		return nil, nil, fmt.Errorf("wire: %w", err)
	}
	shopSvc := shop.NewService(database, sec, cfg.FeatureManualPay)
	pricingSvc := pricing.NewService(database, shopSvc)
	shopSvc.SetRepricer(pricingSvc)
	catalogSvc := catalog.NewService(database, shopSvc, nil, pricingSvc, cfg.ImgBaseURL)
	svc := connector.NewService(database, sec, source.NewPostgres(source.Options{}), cfg.ImgBaseURL).
		Attach(connector.Runtime{Pricer: pricingSvc, Products: catalogSvc, Hours: shopSvc})
	return New(svc), database.Close, nil
}

// Handle is the Lambda handler. The event is never logged: it names a connector
// and nothing else, but the rule is the same for every function.
func (j *Job) Handle(ctx context.Context, raw json.RawMessage) error {
	var job connector.Job
	if err := json.Unmarshal(raw, &job); err != nil || job.ConnectorID == uuid.Nil || !job.Trigger.Valid() {
		logx.From(ctx).Error("syncjob.bad_event")
		return nil // a malformed event can never succeed: retrying it would only repeat the error
	}
	_, err := j.svc.Run(ctx, job)
	if err != nil {
		// The run list holds the outcome; the invoke itself is not retried.
		logx.From(ctx).Warn("syncjob.run_failed", "connector", job.ConnectorID)
	}
	return nil
}
