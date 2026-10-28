// Package site keeps the public web pages current: when a shop is created or
// renamed, the static site is rebuilt through the Cloudflare Pages deploy hook.
package site

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/secrets"
)

const (
	hookSecret  = "pages/deploy-hook"
	hookTimeout = 5 * time.Second
)

// Rebuilder is the consumer of site.rebuild events (D30).
type Rebuilder struct {
	secrets secrets.Getter
	http    *http.Client
	// allowHTTP lets tests use a plain local server. A real hook is always https.
	allowHTTP bool
}

func NewRebuilder(sec secrets.Getter) *Rebuilder {
	return &Rebuilder{secrets: sec, http: &http.Client{Timeout: hookTimeout}}
}

// Register adds the consumer to the registry.
func (r *Rebuilder) Register(reg *events.Registry) {
	reg.On(enum.EventTypeSiteRebuild, events.Consumer{Name: "site.rebuild", Handle: r.OnRebuild})
}

// OnRebuild builds the site once for a burst of changes. The queue delivers the
// event 120 seconds late; if a newer site.rebuild exists by then, that one will
// build, so this one stops. Otherwise it POSTs the deploy hook. A failure is
// returned, so the queue retries and, after three tries, the DLQ alarm fires.
func (r *Rebuilder) OnRebuild(ctx context.Context, tx *db.Tx, m events.Message) error {
	var newer bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM outbox n, outbox me
		  WHERE me.id = $1 AND n.type = $2 AND n.id <> me.id
		    AND (n.created_at, n.id) > (me.created_at, me.id))`, m.ID, enum.EventTypeSiteRebuild).Scan(&newer)
	if err != nil || newer {
		return err
	}
	return r.deploy(ctx)
}

func (r *Rebuilder) deploy(ctx context.Context) error {
	hook, err := r.secrets.Get(ctx, hookSecret)
	if err != nil {
		return fmt.Errorf("site: deploy hook: %w", err)
	}
	u, err := url.Parse(hook)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(r.allowHTTP && u.Scheme == "http")) {
		return fmt.Errorf("site: the deploy hook secret is not an https URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook, http.NoBody)
	if err != nil {
		return fmt.Errorf("site: deploy request: %w", err)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		// The URL holds the hook's secret id, so the error is rewritten without it.
		return fmt.Errorf("site: deploy hook call failed: %s", errKind(err))
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("site: deploy hook answered %d", resp.StatusCode)
	}
	return nil
}

// errKind keeps the reason of a transport failure but drops the URL net/http puts in it.
func errKind(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return err.Error()
}
