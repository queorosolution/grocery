package httpx

import "context"

type ctxKey int

const (
	requestIDKey ctxKey = iota
	routeKey
)

const requestIDHeader = "X-Request-Id"

// RequestID returns the id of the current request, or "" outside a request.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// routeHolder lets the access log learn the matched route pattern, which only
// the router knows, without logging the raw path (it can contain ids).
type routeHolder struct{ pattern string }

// SetRoute records the matched route pattern for the access log.
func SetRoute(ctx context.Context, pattern string) {
	if h, ok := ctx.Value(routeKey).(*routeHolder); ok {
		h.pattern = pattern
	}
}
