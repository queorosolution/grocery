// Package turnstile checks a Cloudflare Turnstile token on the server. The
// browser widget alone proves nothing: anyone can skip it and call the API, so
// routes that cost money (SMS) verify the token here before doing any work.
package turnstile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/secrets"
)

const (
	verifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	secretKey = "turnstile/secret"
	// maxTokenLen is Cloudflare's documented limit; a longer value is junk.
	maxTokenLen = 2048
	timeout     = 5 * time.Second
	maxBody     = 64 << 10
)

// Verifier is what routes depend on, so tests can swap in a fake.
type Verifier interface {
	// Verify returns nil for a good token, BOT_CHECK_FAILED for a refused one.
	Verify(ctx context.Context, token, remoteIP string) error
}

// Client calls Cloudflare. Any failure to get an answer is an error, never a pass.
type Client struct {
	secrets secrets.Getter
	url     string
	http    *http.Client
}

func New(sec secrets.Getter) *Client { return newClient(sec, verifyURL) }

func newClient(sec secrets.Getter, endpoint string) *Client {
	return &Client{secrets: sec, url: endpoint, http: &http.Client{Timeout: timeout}}
}

type answer struct {
	Success bool     `json:"success"`
	Errors  []string `json:"error-codes"`
}

func (c *Client) Verify(ctx context.Context, token, remoteIP string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenLen {
		return apperr.New(apperr.BotCheckFailed)
	}
	secret, err := c.secrets.Get(ctx, secretKey)
	if err != nil {
		return err
	}
	form := url.Values{"secret": {secret}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, strings.NewReader(form.Encode()))
	if err != nil {
		return apperr.New(apperr.Internal).Wrap(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return apperr.New(apperr.Internal).Wrap(fmt.Errorf("turnstile: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return apperr.New(apperr.Internal).Wrap(fmt.Errorf("turnstile: status %d", resp.StatusCode))
	}
	var a answer
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&a); err != nil {
		return apperr.New(apperr.Internal).Wrap(fmt.Errorf("turnstile: bad answer: %w", err))
	}
	if !a.Success {
		return apperr.New(apperr.BotCheckFailed)
	}
	return nil
}
