package http

import (
	"context"
	"fmt"

	"gp/internal/audit"
	"gp/internal/auth"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/secrets"
	"gp/internal/shop"
	"gp/internal/team"
)

// Wire builds the real dependencies for the API: the database, the modules and
// their route handlers. cmd/api and cmd/devserver both call it, so the two
// entry points can never drift apart. The returned func closes the database.
func Wire(ctx context.Context, cfg *config.Config, sec secrets.Store) (Deps, func(), error) {
	database, err := db.Connect(ctx, cfg, sec)
	if err != nil {
		return Deps{}, nil, fmt.Errorf("wire: %w", err)
	}
	teamSvc := team.NewService(database)
	authSvc := auth.NewService(cfg, database, sec, auth.NewGoogleVerifier(cfg.GoogleClientID), teamSvc)
	return Deps{
		Config:      cfg,
		Auth:        auth.NewHandler(authSvc, cfg),
		Team:        team.NewHandler(teamSvc),
		Shop:        shop.NewHandler(shop.NewService(database, sec, cfg.FeatureManualPay)),
		Audit:       audit.NewHandler(database),
		Memberships: teamSvc,
	}, database.Close, nil
}
