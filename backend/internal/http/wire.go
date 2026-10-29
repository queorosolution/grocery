package http

import (
	"context"
	"fmt"

	"gp/internal/audit"
	"gp/internal/auth"
	"gp/internal/cartclient"
	"gp/internal/catalog"
	"gp/internal/connector"
	"gp/internal/connector/connaws"
	"gp/internal/connector/source"
	"gp/internal/coupon"
	"gp/internal/delivery"
	"gp/internal/events"
	"gp/internal/events/eventsaws"
	"gp/internal/finance"
	"gp/internal/notify"
	"gp/internal/notify/logchannel"
	"gp/internal/order"
	"gp/internal/payment"
	"gp/internal/payment/manual"
	"gp/internal/payment/payhere"
	"gp/internal/pricing"
	"gp/internal/refund"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/secrets"
	"gp/internal/shared/turnstile"
	"gp/internal/shop"
	"gp/internal/team"
	"gp/internal/upload"
	"gp/internal/user"
)

// Wire builds the real dependencies for the API: the database, the modules and
// their route handlers. cmd/api and cmd/devserver both call it, so the two
// entry points can never drift apart. The returned func closes the database.
func Wire(ctx context.Context, cfg *config.Config, sec secrets.Store) (Deps, func(), error) {
	database, err := db.Connect(ctx, cfg, sec)
	if err != nil {
		return Deps{}, nil, fmt.Errorf("wire: %w", err)
	}
	// Committed events are sent to the queue right away; without a queue (local
	// runs) they wait for the outbox sweep.
	queue, sched, err := eventsaws.New(ctx, cfg)
	if err != nil {
		database.Close()
		return Deps{}, nil, fmt.Errorf("wire: %w", err)
	}
	if queue != nil {
		events.SetPublisher(events.NewPublisher(database, queue, sched))
	}
	teamSvc := team.NewService(database)
	shopSvc := shop.NewService(database, sec, cfg.FeatureManualPay)
	deliverySvc := delivery.NewService(database, shopSvc)
	pricingSvc := pricing.NewService(database, shopSvc)
	shopSvc.SetRepricer(pricingSvc)
	catalogSvc := catalog.NewService(database, shopSvc, deliverySvc, pricingSvc, cfg.ImgBaseURL)
	carts := cartclient.New(cfg.CartAPIBase, sec, cfg.AllowedOrigins)
	shopSvc.SetCarts(carts, catalogSvc)
	schedules, invoker, err := connaws.New(ctx, cfg)
	if err != nil {
		database.Close()
		return Deps{}, nil, fmt.Errorf("wire: %w", err)
	}
	connectorSvc := connector.NewService(database, sec, source.NewPostgres(source.Options{}), cfg.ImgBaseURL).
		Attach(connector.Runtime{Pricer: pricingSvc, Products: catalogSvc, Hours: shopSvc, Schedules: schedules, Invoker: invoker})
	connectorHandler := connector.NewHandler(connectorSvc)
	uploadSvc := upload.NewService(upload.NewR2(cfg.R2AccountID, cfg.R2Bucket, sec), cfg.ImgBaseURL)
	sms := notify.New(logchannel.New(cfg.IsLocal())) // real SMS comes with P25
	userSvc := user.NewService(database, sec, turnstile.New(sec), sms, deliverySvc, cfg.PlatformName)
	authSvc := auth.NewService(cfg, database, sec, auth.NewGoogleVerifier(cfg.GoogleClientID), teamSvc)
	couponSvc := coupon.NewService(database)
	orderSvc := order.NewService(database, shopSvc, deliverySvc, userSvc, catalogSvc, carts, couponSvc, finance.NewService())
	paymentHandler := payment.NewHandler(payment.NewService(database, orderSvc, shopSvc, refund.NewService(), payment.Registry{
		enum.PaymentProviderPayhere: payhere.New(sec, cfg.WebOrigin, cfg.APIBaseURL+Base+payment.NotifyPattern),
		enum.PaymentProviderManual:  manual.New(),
	}))
	return Deps{
		Config:      cfg,
		Auth:        auth.NewHandler(authSvc, cfg),
		Team:        team.NewHandler(teamSvc),
		Shop:        shop.NewHandler(shopSvc),
		Delivery:    delivery.NewHandler(deliverySvc),
		User:        user.NewHandler(userSvc, authSvc),
		Catalog:     catalog.NewHandler(catalogSvc),
		Pricing:     pricing.NewHandler(pricingSvc),
		Upload:      upload.NewHandler(uploadSvc, teamSvc),
		Order:       order.NewHandler(orderSvc),
		Coupon:      coupon.NewHandler(couponSvc),
		Payment:     paymentHandler,
		Connector:   connectorHandler,
		Audit:       audit.NewHandler(database),
		Memberships: teamSvc,
		VerifySig: SigByRoute(map[string]SigVerifier{
			"POST " + Base + TriggerPattern:        connectorHandler.VerifyTrigger,
			"POST " + Base + payment.NotifyPattern: paymentHandler.VerifyNotify,
		}),
	}, database.Close, nil
}
