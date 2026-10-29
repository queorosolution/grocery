package worker

import (
	"context"
	"fmt"

	"gp/internal/cartclient"
	"gp/internal/catalog"
	"gp/internal/events"
	"gp/internal/events/eventsaws"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
	"gp/internal/shop"
	"gp/internal/site"
)

// Wire builds the worker and returns a function that closes what it opened.
// Adding a consumer or a job is one line in this file.
func Wire(ctx context.Context, cfg *config.Config, sec secrets.Store) (*Worker, func(), error) {
	database, err := db.Connect(ctx, cfg, sec)
	if err != nil {
		return nil, nil, fmt.Errorf("wire: %w", err)
	}
	queue, sched, err := eventsaws.New(ctx, cfg)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("wire: %w", err)
	}
	if queue == nil {
		database.Close()
		return nil, nil, fmt.Errorf("wire: EVENTS_QUEUE_URL is not set")
	}
	publisher := events.NewPublisher(database, queue, sched)

	registry := events.NewRegistry()
	site.NewRebuilder(sec).Register(registry)
	catalog.NewCartSync(shop.NewService(database, sec, cfg.FeatureManualPay), cartclient.New(cfg.CartAPIBase, sec, cfg.AllowedOrigins)).
		Register(registry)
	catalog.NewBuyAgain().Register(registry)
	// The other consumers (notify, stats) register here when their modules arrive.

	return New(database, registry, Jobs(publisher)), database.Close, nil
}

// Jobs are the scheduled tasks, by the "type" of the direct invoke.
func Jobs(publisher *events.Publisher) map[string]Job {
	return map[string]Job{
		"outbox.sweep": func(ctx context.Context) error {
			res, err := publisher.Sweep(ctx)
			logx.From(ctx).Info("worker.outbox_sweep", "found", res.Found, "sent", res.Sent, "pruned", res.Pruned)
			return err
		},
		// The daily stats roll-up arrives with the stats module; until then the
		// schedule fires and finds nothing to close.
		"stats.rollup": func(ctx context.Context) error {
			logx.From(ctx).Info("worker.stats_rollup", "closed", 0)
			return nil
		},
	}
}
