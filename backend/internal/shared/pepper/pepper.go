// Package pepper hashes identifiers (IP addresses, phone numbers, OTP codes)
// with a secret key from SSM, so the database and the rate counters never hold
// the raw value and a stolen copy cannot be reversed by guessing.
package pepper

import (
	"context"

	"gp/internal/shared/crypt"
	"gp/internal/shared/secrets"
)

// Names of the peppers in docs/13 section 2.
const (
	IP  = "ip/pepper"
	OTP = "otp/pepper"
)

// Hash returns HMAC-SHA256 of the parts, keyed by the named pepper.
func Hash(ctx context.Context, sec secrets.Getter, name string, parts ...string) ([]byte, error) {
	key, err := sec.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	raw := make([][]byte, len(parts))
	for i, p := range parts {
		raw[i] = []byte(p)
	}
	return crypt.HMAC([]byte(key), raw...), nil
}
