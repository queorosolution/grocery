package connector

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ratelimit"
	"gp/internal/shared/validate"
)

const (
	// triggerWindow is how far the signed time may be from ours (docs/06 section 7).
	triggerWindow = 5 * time.Minute
	// triggerLimit calls an hour per connector (docs/06 section 1).
	triggerLimit = 30
	// maxTriggerIDs is the most external ids one call may name.
	maxTriggerIDs = 100

	triggerSecretSuffix = "/trigger" // SSM name: connectors/<id>/trigger (docs/13 section 2)
	signaturePrefix     = "v1="
)

// TriggerSecret is shown once, when it is made.
type TriggerSecret struct {
	Secret  string `json:"secret"`
	Example string `json:"example"` // a signed call the shop can copy
}

// RotateTriggerSecret makes a new trigger secret and forgets the old one: the
// shop's system must be given the new one before its next call. The value goes
// to the secret store and into this one answer; the audit row records only that
// it was rotated.
func (s *Service) RotateTriggerSecret(ctx context.Context, scope db.Scope, id uuid.UUID) (TriggerSecret, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return TriggerSecret{}, apperr.New(apperr.Internal).Wrap(err)
	}
	secret := "gpt_" + base64.RawURLEncoding.EncodeToString(raw)
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := byID(ctx, tx, id, true)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		ref := secretPrefix + id.String() + triggerSecretSuffix
		if err := s.secrets.Put(ctx, ref, secret); err != nil {
			return apperr.New(apperr.Internal).Wrap(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE connectors SET trigger_secret_ref = $2 WHERE id = $1`, id, ref); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "connector.trigger_secret.rotate", Entity: "connector", EntityID: id.String(), ShopID: before.ShopID,
			Before: map[string]any{"hadSecret": before.HasTriggerSecret}, After: map[string]any{"hadSecret": true},
		})
	})
	if err != nil {
		return TriggerSecret{}, err
	}
	return TriggerSecret{Secret: secret, Example: triggerExample(id)}, nil
}

// triggerExample is a shell snippet that signs and sends one call. It names the
// secret by variable, so the answer holds the secret only once.
func triggerExample(id uuid.UUID) string {
	return `ts=$(date +%s); body='{"externalIds":["12345"]}'
sig=$(printf '%s.%s' "$ts" "$body" | openssl dgst -sha256 -hmac "$TRIGGER_SECRET" -hex | sed 's/^.* //')
curl -X POST "$API_BASE/v1/connectors/` + id.String() + `/trigger" \
  -H "Content-Type: application/json" -H "X-GP-Timestamp: $ts" -H "X-GP-Signature: v1=$sig" -d "$body"`
}

// Authenticate checks a signed trigger call (BIZ-16): the time is within five
// minutes, the HMAC-SHA256 of "<timestamp>.<raw body>" matches, and the
// connector has not been called more than 30 times this hour. Every failure is
// the same UNAUTHENTICATED, so the answer does not tell whether the connector
// exists. Only calls with a good signature count against the limit, so someone
// without the secret cannot use up the shop's hourly calls.
func (s *Service) Authenticate(ctx context.Context, id uuid.UUID, timestamp, signature string, body []byte) error {
	denied := apperr.New(apperr.Unauthenticated)
	sent, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return denied
	}
	if d := clock.Now().Sub(time.Unix(sent, 0)); d > triggerWindow || d < -triggerWindow {
		return denied
	}
	want, ok := strings.CutPrefix(signature, signaturePrefix)
	got, err := hex.DecodeString(want)
	if !ok || err != nil || len(got) != sha256.Size {
		return denied
	}
	var ref *string
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		return tx.QueryRow(ctx, `SELECT trigger_secret_ref FROM connectors WHERE id = $1`, id).Scan(&ref)
	})
	if err != nil || ref == nil {
		return denied
	}
	secret, err := s.secrets.Get(ctx, *ref)
	if err != nil {
		return denied
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), got) {
		return denied
	}
	return ratelimit.Hit(ctx, s.db, "connector-trigger:"+id.String(), triggerLimit, time.Hour)
}

// Trigger starts a run for a call that Authenticate passed, without waiting for
// it. With external ids only those rows are read; without, it is an incremental
// run. A connector that is not live answers CONFLICT, so the shop's system sees
// that nothing will happen.
func (s *Service) Trigger(ctx context.Context, id uuid.UUID, externalIDs []string) error {
	v := validate.New().Check("externalIds", len(externalIDs) <= maxTriggerIDs, "too_many")
	for _, e := range externalIDs {
		if e == "" || len(e) > maxExternalID {
			v.Check("externalIds", false, "invalid")
		}
	}
	if err := v.Err(); err != nil {
		return err
	}
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		c, found, err := byID(ctx, tx, id, false)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if c.Status != enum.ConnectorStatusActive && c.Status != enum.ConnectorStatusFailing {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"status": "not_active"}}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.start(ctx, Job{ConnectorID: id, Trigger: enum.RunTriggerWebhook, ExternalIDs: externalIDs})
}
