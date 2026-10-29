package db

import (
	"context"
	"errors"
	"fmt"

	"gp/internal/shared/config"
	"gp/internal/shared/secrets"
)

// secretName is the SSM name of the app_rw connection string (docs/13 section 2).
const secretName = "db/url"

// Connect opens the application database. Local runs may use DATABASE_URL;
// everywhere else the URL comes from the secret store and TLS plus the app_rw
// role are enforced.
func Connect(ctx context.Context, cfg *config.Config, s secrets.Getter) (*DB, error) {
	url, err := databaseURL(ctx, cfg, s)
	if err != nil {
		return nil, err
	}
	return Open(ctx, Options{URL: url, Local: cfg.IsLocal()})
}

func databaseURL(ctx context.Context, cfg *config.Config, s secrets.Getter) (string, error) {
	if cfg.IsLocal() && cfg.DatabaseURL != "" {
		return cfg.DatabaseURL, nil
	}
	if s == nil {
		return "", errors.New("db: no DATABASE_URL and no secret store")
	}
	url, err := s.Get(ctx, secretName)
	if err != nil {
		return "", fmt.Errorf("db: read %s: %w", secretName, err)
	}
	return url, nil
}
