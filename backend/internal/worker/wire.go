package worker

import (
	"context"
	"fmt"

	"gp/internal/cartclient"
	"gp/internal/catalog"
	"gp/internal/events"
	"gp/internal/events/eventsaws"
	"gp/internal/notify/consumer"
	notifysetup "gp/internal/notify/setup"
	"gp/internal/order"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
	"gp/internal/shop"
	"gp/internal/site"
	"gp/internal/stats"
	"gp/internal/user"
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

	notifySvc, err := notifysetup.Service(cfg, database, sec)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("wire: %w", err)
	}
	shopSvc := shop.NewService(database, sec, cfg.FeatureManualPay)

	registry := events.NewRegistry()
	site.NewRebuilder(sec).Register(registry)
	catalog.NewCartSync(shopSvc, cartclient.New(cfg.CartAPIBase, sec, cfg.AllowedOrigins)).Register(registry)
	catalog.NewBuyAgain().Register(registry)
	// The texts only read: the order service needs the database and the key that
	// seals delivery codes, and the user service needs nothing but the database.
	orderSvc := order.NewService(order.Deps{DB: database, Secrets: sec})
	consumer.New(notifySvc, orderSvc, shopSvc, user.NewService(database, sec, nil, nil, nil, ""),
		consumer.Config{App: cfg.PlatformName, Domain: cfg.PlatformDomain, AdminPhone: cfg.AdminAlertPhone}).Register(registry)
	statsSvc := stats.NewService(database, orderSvc)
	statsSvc.Register(registry)

	return New(database, registry, Jobs(publisher, statsSvc)), database.Close, nil
}

// Jobs are the scheduled tasks, by the "type" of the direct invoke.
func Jobs(publisher *events.Publisher, statsSvc *stats.Service) map[string]Job {
	return map[string]Job{
		"outbox.sweep": func(ctx context.Context) error {
			res, err := publisher.Sweep(ctx)
			logx.From(ctx).Info("worker.outbox_sweep", "found", res.Found, "sent", res.Sent, "pruned", res.Pruned)
			return err
		},
		// The nightly job: it deletes what is past its retention (docs/05 section 7).
		"stats.rollup": func(ctx context.Context) error {
			deleted, err := statsSvc.Rollup(ctx)
			logx.From(ctx).Info("worker.stats_rollup", "deleted", deleted)
			return err
		},
	}
}
