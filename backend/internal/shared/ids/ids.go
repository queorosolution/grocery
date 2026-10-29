// Package ids makes identifiers. Everything random comes from crypto/rand.
package ids

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"

	"github.com/oklog/ulid/v2"

	"gp/internal/shared/clock"
)

// crockford has no I, L, O or U, so order codes are easy to read out loud.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const orderCodeLen = 6

var (
	mu      sync.Mutex
	entropy = ulid.Monotonic(rand.Reader, 0) // not safe for concurrent use, so guarded by mu
)

// Event returns a sortable id for an outbox event: evt_<ULID>.
func Event() string {
	mu.Lock()
	defer mu.Unlock()
	return "evt_" + ulid.MustNew(ulid.Timestamp(clock.Now()), entropy).String()
}

// OrderCode returns <PREFIX>-<6 Crockford chars>, e.g. KS-7Q2M9X. The caller
// retries once on a unique-index clash (docs/07 §9).
func OrderCode(shopPrefix string) string {
	return ShopPrefix(shopPrefix) + "-" + randomFrom(crockford, orderCodeLen)
}

// ShopPrefix takes the first two letters of a shop slug, upper case. A slug
// with fewer than two letters gets "X" padding so the code shape never changes.
func ShopPrefix(slug string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(slug) {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
			if b.Len() == 2 {
				break
			}
		}
	}
	for b.Len() < 2 {
		b.WriteByte('X')
	}
	return b.String()
}

// Token returns 32 random bytes as URL-safe base64 (43 characters).
func Token() string {
	b := make([]byte, 32)
	rand.Read(b) // never fails since Go 1.24: it crashes the program instead
	return base64.RawURLEncoding.EncodeToString(b)
}

// randomFrom picks n symbols from a 32-symbol alphabet. 256 is a multiple of
// 32, so masking a byte gives every symbol the same chance.
func randomFrom(alphabet string, n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[b[i]&31]
	}
	return string(b)
}
