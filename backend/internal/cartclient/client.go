// Package cartclient talks to the shared cart service (docs/09 section 1). The
// core calls it with the platform key and, for tenant routes, X-Tenant-Id. The
// browser uses the shop's public key and never goes through here.
package cartclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gp/internal/shared/secrets"
)

const (
	// requestTimeout is the longest one call may take (docs/09): the shopper waits.
	requestTimeout = 3 * time.Second
	keySecret      = "cart/platform-key" // #nosec G101 -- the name of the SSM parameter, not a credential
	maxBody        = 1 << 20
)

// retryDelays are the waits before the 2nd and 3rd try of a call that failed
// with a network error, a 5xx or a rate limit.
var retryDelays = []time.Duration{150 * time.Millisecond, 400 * time.Millisecond}

type Client struct {
	runtime, admin string // base URLs, no trailing slash; both empty when not configured
	secrets        secrets.Getter
	http           *http.Client
	delays         []time.Duration
	origins        []string // allowedOrigins for new tenants
}

// New builds a client. base is CART_API_BASE (for example
// https://api.cart.example.com/v1); the admin API lives beside it at /admin/v1.
// allowedOrigins are given to every new tenant, so its public key works from
// our website only.
func New(base string, sec secrets.Getter, allowedOrigins []string) *Client {
	c := &Client{secrets: sec, origins: allowedOrigins, http: &http.Client{Timeout: requestTimeout}, delays: retryDelays}
	if base = strings.TrimRight(base, "/"); base != "" {
		c.runtime, c.admin = base, strings.TrimSuffix(base, "/v1")+"/admin/v1"
	}
	return c
}

// WithoutDelays makes retries immediate, for tests.
func (c *Client) WithoutDelays() *Client {
	c.delays = make([]time.Duration, len(retryDelays))
	return c
}

// call is one request in the shape the cart service expects.
type call struct {
	op, method, url string
	tenant          string // X-Tenant-Id, empty for admin routes
	body            any
	out             any // decoded from a 2xx body when not nil
}

// do sends the request with the platform key and retries it when the service
// fails on its side. A refusal (4xx) is final.
func (c *Client) do(ctx context.Context, k call) error {
	if c.runtime == "" {
		return &Error{Op: k.op, kind: ErrUnavailable}
	}
	key, err := c.secrets.Get(ctx, keySecret)
	if err != nil || key == "" {
		return &Error{Op: k.op, kind: ErrUnavailable} // the cause could hold a secret name: leave it out
	}
	var payload []byte
	if k.body != nil {
		if payload, err = json.Marshal(k.body); err != nil {
			return fmt.Errorf("cartclient: %s: %w", k.op, err)
		}
	}
	var last error
	for attempt := 0; attempt <= len(c.delays); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return &Error{Op: k.op, kind: ErrUnavailable}
			case <-time.After(c.delays[attempt-1]):
			}
		}
		last = c.once(ctx, k, key, payload)
		if last == nil || !retryable(last) {
			return last
		}
	}
	return last
}

func retryable(err error) bool {
	var e *Error
	return errors.As(err, &e) && errors.Is(e, ErrUnavailable)
}

func (c *Client) once(ctx context.Context, k call, key string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, k.method, k.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cartclient: %s: bad request", k.op)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if k.tenant != "" {
		req.Header.Set("X-Tenant-Id", k.tenant)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// The error text holds the URL, which can hold ids: report the kind only.
		return &Error{Op: k.op, kind: ErrUnavailable}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if k.out != nil && json.Unmarshal(raw, k.out) != nil {
			return &Error{Op: k.op, Status: resp.StatusCode, kind: ErrRejected}
		}
		return nil
	}
	return failure(k.op, resp.StatusCode, raw)
}

// failure classifies an error answer. Only the code and details.tenantId of the
// body are read; the message is not kept.
func failure(op string, status int, raw []byte) *Error {
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				TenantID string `json:"tenantId"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &body)
	e := &Error{Op: op, Status: status, Code: body.Error.Code, TenantID: body.Error.Details.TenantID}
	switch {
	case status == http.StatusNotFound:
		e.kind = ErrNotFound
	case status == http.StatusConflict && e.Code == "CONFLICT":
		e.kind = ErrConflict
	case e.Code == "QUOTA_EXCEEDED":
		e.kind = ErrQuota
	case status >= 500 || status == http.StatusTooManyRequests || status == http.StatusRequestTimeout:
		e.kind = ErrUnavailable
	default:
		e.kind = ErrRejected
	}
	return e
}

// path joins escaped path parts to a base.
func path(base string, parts ...string) string {
	for _, p := range parts {
		base += "/" + url.PathEscape(p)
	}
	return base
}
