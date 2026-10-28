package http

import (
	"context"
	"fmt"

	"gp/internal/auth"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/secrets"
)

// Wire builds the real dependencies for the API: the database, the modules and
// their route handlers. cmd/api and cmd/devserver both call it, so the two
// entry points can never drift apart. The returned func closes the database.
func Wire(ctx context.Context, cfg *config.Config, sec secrets.Getter) (Deps, func(), error) {
	database, err := db.Connect(ctx, cfg, sec)
	if err != nil {
		return Deps{}, nil, fmt.Errorf("wire: %w", err)
	}
	authSvc := auth.NewService(cfg, database, sec, auth.NewGoogleVerifier(cfg.GoogleClientID))
	return Deps{
		Config: cfg,
		Auth:   auth.NewHandler(authSvc, cfg),
	}, database.Close, nil
}
