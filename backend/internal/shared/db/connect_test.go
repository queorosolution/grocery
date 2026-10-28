package db

import (
	"context"
	"errors"
	"testing"

	"gp/internal/shared/config"
)

type fakeSecrets map[string]string

func (f fakeSecrets) Get(_ context.Context, name string) (string, error) {
	if v, ok := f[name]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func TestDatabaseURLSource(t *testing.T) {
	ctx := context.Background()
	store := fakeSecrets{"db/url": "from-secret"}
	cases := []struct {
		name string
		cfg  *config.Config
		s    fakeSecrets
		want string
		fail bool
	}{
		{"local prefers DATABASE_URL", &config.Config{Env: config.EnvLocal, DatabaseURL: "from-env"}, store, "from-env", false},
		{"local falls back to the secret", &config.Config{Env: config.EnvLocal}, store, "from-secret", false},
		{"dev reads the secret", &config.Config{Env: config.EnvDev}, store, "from-secret", false},
		// Config refuses DATABASE_URL outside local; this proves Connect would also ignore it.
		{"prod ignores DATABASE_URL", &config.Config{Env: config.EnvProd, DatabaseURL: "from-env"}, store, "from-secret", false},
		{"missing secret fails", &config.Config{Env: config.EnvProd}, fakeSecrets{}, "", true},
	}
	for _, tc := range cases {
		got, err := databaseURL(ctx, tc.cfg, tc.s)
		if (err != nil) != tc.fail || got != tc.want {
			t.Errorf("%s: got %q, %v", tc.name, got, err)
		}
	}
}
