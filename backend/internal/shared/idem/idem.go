// Package idem makes a retried POST safe (docs/06 section 1). The key is claimed
// in the same transaction as the work, so the work and its key commit or roll
// back together: a failed request leaves no key behind and may be retried, and
// a request that succeeded is answered again with the same response.
package idem

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
)

// keepFor is how long a key answers a retry. The sweep removes older rows later.
const keepFor = 24 * time.Hour

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// Stored is the answer of the first request with a key.
type Stored struct {
	Status int
	Body   json.RawMessage
}

// Key reads the Idempotency-Key header. Both a missing and a badly shaped key are
// a VALIDATION_ERROR; the key itself is never echoed back.
func Key(r *http.Request) (string, error) {
	k := r.Header.Get("Idempotency-Key")
	switch {
	case k == "":
		return "", apperr.Validation(map[string]string{"Idempotency-Key": "required"})
	case !keyPattern.MatchString(k):
		return "", apperr.Validation(map[string]string{"Idempotency-Key": "invalid"})
	}
	return k, nil
}

// Hash is the fingerprint of a request body: the same input always gives the
// same bytes, so a retry matches and a changed body does not.
func Hash(body any) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}

// Lookup answers a retry. It returns nil when the key was not used (or has
// expired), the stored answer when it was used with the same body, and
// IDEMPOTENCY_MISMATCH when it was used with a different body.
func Lookup(ctx context.Context, tx *db.Tx, userID uuid.UUID, route, key string, hash []byte) (*Stored, error) {
	var (
		prior  []byte
		status *int
		body   []byte
		madeAt time.Time
	)
	err := tx.QueryRow(ctx, `SELECT body_hash, status_code, response, created_at FROM idempotency_keys
		WHERE user_id = $1 AND route = $2 AND key = $3`, userID, route, key).Scan(&prior, &status, &body, &madeAt)
	if err != nil || clock.Now().Sub(madeAt) > keepFor {
		return nil, db.IgnoreNoRows(err)
	}
	if string(prior) != string(hash) {
		return nil, apperr.New(apperr.IdempotencyMismatch)
	}
	if status == nil {
		return nil, apperr.New(apperr.Conflict) // claimed but never finished: cannot happen inside one transaction
	}
	return &Stored{Status: *status, Body: body}, nil
}

// Claim takes the key inside the work's transaction. It returns the stored
// answer when another request got there first; the caller then returns it and
// does nothing else. A request with the same key waits here until the first
// one commits or rolls back.
func Claim(ctx context.Context, tx *db.Tx, userID uuid.UUID, route, key string, hash []byte) (*Stored, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM idempotency_keys WHERE user_id = $1 AND route = $2 AND key = $3 AND created_at < $4`,
		userID, route, key, clock.Now().Add(-keepFor)); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO idempotency_keys (user_id, route, key, body_hash) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, userID, route, key, hash)
	if err != nil || tag.RowsAffected() == 1 {
		return nil, err
	}
	return Lookup(ctx, tx, userID, route, key, hash)
}

// Save records the answer for later retries.
func Save(ctx context.Context, tx *db.Tx, userID uuid.UUID, route, key string, status int, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE idempotency_keys SET status_code = $4, response = $5::jsonb
		WHERE user_id = $1 AND route = $2 AND key = $3`, userID, route, key, status, string(raw))
	return err
}
