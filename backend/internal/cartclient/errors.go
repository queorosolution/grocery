package cartclient

import (
	"errors"
	"fmt"

	"gp/internal/shared/apperr"
)

// The kinds of failure a caller can act on. Use errors.Is to test for them.
var (
	// ErrUnavailable: the cart service cannot be reached or is failing (timeout,
	// network error, 5xx, rate limit) even after the retries.
	ErrUnavailable = errors.New("cartclient: cart service unavailable")
	// ErrNotFound: the cart, tenant or item does not exist (for this tenant).
	ErrNotFound = errors.New("cartclient: not found")
	// ErrConflict: the thing already exists (a tenant for this external id).
	ErrConflict = errors.New("cartclient: conflict")
	// ErrQuota: the tenant's plan does not allow more items.
	ErrQuota = errors.New("cartclient: item quota reached")
	// ErrRejected: the cart service refused the request (4xx not listed above).
	// Repeating it will not help.
	ErrRejected = errors.New("cartclient: request refused")
)

// Error is a failed call. It never holds a key, a request body or a response
// body: only the operation, the HTTP status and the cart service's error code.
type Error struct {
	Op     string // "items.batch"
	Status int    // HTTP status, 0 when no answer came
	Code   string // the cart service's error.code, when it sent one
	// TenantID is details.tenantId of a CONFLICT: the tenant that already has the id.
	TenantID string
	kind     error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %v (status %d, code %q)", e.Op, e.kind, e.Status, e.Code)
}

func (e *Error) Unwrap() error { return e.kind }

// ToAppError turns an unreachable cart service into CART_UNAVAILABLE for the
// caller's client, and leaves every other error as it is.
func ToAppError(err error) error {
	if errors.Is(err, ErrUnavailable) {
		return apperr.New(apperr.CartUnavailable).Wrap(err)
	}
	return err
}
