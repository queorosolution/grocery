// Package secrets reads secrets from SSM SecureString with a short cache. Only
// APP_ENV=local may read them from SECRET_<NAME> variables instead (docs/13-config.md).
// Values are never logged and never put in error messages.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"gp/internal/shared/clock"
	"gp/internal/shared/config"
)

// TTL is how long a value is reused. It lets a rotated secret take effect on a
// warm Lambda within minutes while keeping SSM calls (and cost) low.
const TTL = 5 * time.Minute

var ErrNotFound = errors.New("secrets: not found")

// nameRe keeps names to the shape in docs/13 §2, e.g. db/url or payhere/<id>.
var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*(/[a-z0-9][a-z0-9_-]*)*$`)

// ParameterAPI is the part of the SSM client we use, so tests can fake it.
type ParameterAPI interface {
	GetParameter(ctx context.Context, in *ssm.GetParameterInput, opts ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	PutParameter(ctx context.Context, in *ssm.PutParameterInput, opts ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

type Getter interface {
	Get(ctx context.Context, name string) (string, error)
}

// Store reads and writes secrets; *Client is the real one.
type Store interface {
	Getter
	Putter
}

// Putter writes secrets that the admin API owns (PayHere merchant secrets,
// connector connection strings). The value is never returned by any API.
type Putter interface {
	Put(ctx context.Context, name, value string) error
}

type entry struct {
	value   string
	expires time.Time
}

type Client struct {
	prefix  string
	api     ParameterAPI
	environ func(string) string // set only in local env-mode
	local   map[string]string   // values written in local env-mode, kept in memory only

	mu    sync.Mutex
	cache map[string]entry
}

// New builds the client the config asks for. In local env-mode no SSM client is
// needed; everywhere else api must be set.
func New(cfg *config.Config, api ParameterAPI, getenv func(string) string) (*Client, error) {
	c := &Client{prefix: cfg.SSMPrefix, cache: map[string]entry{}, local: map[string]string{}}
	if cfg.IsLocal() && cfg.SecretsFromEnv {
		c.environ = getenv
		return c, nil
	}
	if api == nil {
		return nil, errors.New("secrets: an SSM client is required outside local env-mode")
	}
	c.api = api
	return c, nil
}

// NewSSM creates the real SSM client from the default AWS configuration.
func NewSSM(ctx context.Context) (ParameterAPI, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("secrets: load AWS config: %w", err)
	}
	return ssm.NewFromConfig(awsCfg), nil
}

// Get returns the secret called name (for example "jwt/key").
func (c *Client) Get(ctx context.Context, name string) (string, error) {
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("secrets: invalid name %q", name)
	}
	if c.environ != nil {
		return c.fromEnv(name)
	}
	now := clock.Now()
	c.mu.Lock()
	e, ok := c.cache[name]
	c.mu.Unlock()
	if ok && now.Before(e.expires) {
		return e.value, nil
	}
	out, err := c.api.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(c.prefix + "/" + name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		var missing *ssmtypes.ParameterNotFound
		if errors.As(err, &missing) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return "", fmt.Errorf("secrets: read %s: %w", name, err)
	}
	value := aws.ToString(out.Parameter.Value)
	c.mu.Lock()
	c.cache[name] = entry{value: value, expires: now.Add(TTL)}
	c.mu.Unlock()
	return value, nil
}

// Put stores a SecureString parameter, replacing any earlier value, and drops
// the cached copy so the next read sees it. In local env-mode the value stays in
// memory only.
func (c *Client) Put(ctx context.Context, name, value string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("secrets: invalid name %q", name)
	}
	if value == "" {
		return errors.New("secrets: empty value")
	}
	if c.environ != nil {
		c.mu.Lock()
		c.local[name] = value
		c.mu.Unlock()
		return nil
	}
	if _, err := c.api.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(c.prefix + "/" + name),
		Value:     aws.String(value),
		Type:      ssmtypes.ParameterTypeSecureString,
		Overwrite: aws.Bool(true),
	}); err != nil {
		return fmt.Errorf("secrets: write %s: %w", name, err) // the value is not in the error
	}
	c.mu.Lock()
	delete(c.cache, name)
	c.mu.Unlock()
	return nil
}

func (c *Client) fromEnv(name string) (string, error) {
	c.mu.Lock()
	v, ok := c.local[name]
	c.mu.Unlock()
	if ok {
		return v, nil
	}
	key := "SECRET_" + strings.ToUpper(strings.NewReplacer("/", "_", "-", "_").Replace(name))
	if v := c.environ(key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%w: %s (set %s)", ErrNotFound, name, key)
}
