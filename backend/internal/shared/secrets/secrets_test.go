package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"gp/internal/shared/clock"
	"gp/internal/shared/config"
)

type fakeSSM struct {
	values map[string]string
	calls  int
	err    error
	last   *ssm.GetParameterInput
}

func (f *fakeSSM) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	f.calls++
	f.last = in
	if f.err != nil {
		return nil, f.err
	}
	v, ok := f.values[aws.ToString(in.Name)]
	if !ok {
		return nil, &ssmtypes.ParameterNotFound{}
	}
	return &ssm.GetParameterOutput{Parameter: &ssmtypes.Parameter{Value: aws.String(v)}}, nil
}

func cfg(env config.Env, fromEnv bool) *config.Config {
	return &config.Config{Env: env, SSMPrefix: "/gp/dev", SecretsFromEnv: fromEnv}
}

var t0 = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func TestReadsDecryptedSecretFromSSMUnderThePrefix(t *testing.T) {
	defer clock.Freeze(t0)()
	api := &fakeSSM{values: map[string]string{"/gp/dev/jwt/key": "s3cret"}}
	c, _ := New(cfg(config.EnvDev, false), api, nil)
	got, err := c.Get(context.Background(), "jwt/key")
	if err != nil || got != "s3cret" {
		t.Fatalf("got %q, %v", got, err)
	}
	if !aws.ToBool(api.last.WithDecryption) {
		t.Fatal("SecureString must be read with decryption")
	}
}

func TestCachesForFiveMinutes(t *testing.T) {
	api := &fakeSSM{values: map[string]string{"/gp/dev/db/url": "postgres://x"}}
	c, _ := New(cfg(config.EnvDev, false), api, nil)
	ctx := context.Background()

	defer clock.Freeze(t0)()
	c.Get(ctx, "db/url")
	restore := clock.Freeze(t0.Add(TTL - time.Second))
	c.Get(ctx, "db/url")
	restore()
	if api.calls != 1 {
		t.Fatalf("SSM called %d times inside the TTL, want 1", api.calls)
	}

	api.values["/gp/dev/db/url"] = "postgres://rotated"
	defer clock.Freeze(t0.Add(TTL))()
	got, _ := c.Get(ctx, "db/url")
	if api.calls != 2 || got != "postgres://rotated" {
		t.Fatalf("expired value not refreshed: calls=%d got=%q", api.calls, got)
	}
}

func TestErrorsAreNotCached(t *testing.T) {
	defer clock.Freeze(t0)()
	api := &fakeSSM{values: map[string]string{}}
	c, _ := New(cfg(config.EnvDev, false), api, nil)
	if _, err := c.Get(context.Background(), "jwt/key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	api.values["/gp/dev/jwt/key"] = "now-there"
	if got, err := c.Get(context.Background(), "jwt/key"); err != nil || got != "now-there" {
		t.Fatalf("a failed read was cached: %q, %v", got, err)
	}
}

func TestOtherSSMErrorsAreWrappedWithoutValues(t *testing.T) {
	api := &fakeSSM{err: errors.New("throttled")}
	c, _ := New(cfg(config.EnvDev, false), api, nil)
	_, err := c.Get(context.Background(), "jwt/key")
	if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "throttled") {
		t.Fatalf("got %v", err)
	}
}

func TestLocalEnvModeReadsSecretVariables(t *testing.T) {
	env := map[string]string{"SECRET_CART_PLATFORM_KEY": "plk_test", "SECRET_JWT_KEY": "k"}
	c, err := New(cfg(config.EnvLocal, true), nil, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Get(context.Background(), "cart/platform-key"); got != "plk_test" {
		t.Fatalf("got %q", got)
	}
	if _, err := c.Get(context.Background(), "otp/pepper"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "SECRET_OTP_PEPPER") {
		t.Fatalf("a missing local secret should say which variable to set, got %v", err)
	}
}

// Outside local the env fallback must be impossible, even if the variable is set.
func TestEnvFallbackIsNeverUsedOutsideLocal(t *testing.T) {
	api := &fakeSSM{values: map[string]string{"/gp/dev/jwt/key": "from-ssm"}}
	called := false
	getenv := func(string) string { called = true; return "from-env" }
	for _, env := range []config.Env{config.EnvDev, config.EnvProd} {
		c, err := New(cfg(env, true), api, getenv) // SecretsFromEnv=true must be ignored here
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := c.Get(context.Background(), "jwt/key"); got != "from-ssm" || called {
			t.Fatalf("%s read the environment", env)
		}
	}
}

func TestNeedsSSMClientOutsideEnvMode(t *testing.T) {
	if _, err := New(cfg(config.EnvProd, false), nil, nil); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := New(cfg(config.EnvLocal, false), nil, nil); err == nil {
		t.Fatal("local without env-mode also needs SSM")
	}
}

func TestRejectsBadNames(t *testing.T) {
	c, _ := New(cfg(config.EnvDev, false), &fakeSSM{}, nil)
	for _, name := range []string{"", "../etc", "JWT/key", "jwt//key", "/jwt/key", "jwt/key/", "a b"} {
		if _, err := c.Get(context.Background(), name); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
}
