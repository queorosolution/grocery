package httpx

import (
	"context"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/clock"
	"gp/internal/shared/ids"
	"gp/internal/shared/logx"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware so the first one is the outermost: Chain(h, a, b)
// runs a, then b, then h.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

// WithRequestID gives every request a fresh id (a client-sent one is never trusted),
// returns it in X-Request-Id and puts it on the logger.
func WithRequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ids.Request()
			w.Header().Set(requestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			ctx = logx.With(ctx, logx.From(ctx).With("requestId", id))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recover turns a panic into a generic 500 and logs the stack with the request id.
func Recover() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				if p == http.ErrAbortHandler {
					panic(p)
				}
				logx.From(r.Context()).Error("panic in handler", "panic", p, "stack", string(debug.Stack()))
				Error(w, r, apperr.New(apperr.Internal))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter remembers the status code for the access log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog writes one line per request: method, route pattern, status and
// duration. Never the path, query, headers or body: they hold ids and personal data.
func AccessLog() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := clock.Now()
			holder := &routeHolder{}
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), routeKey, holder)))
			route := holder.pattern
			if route == "" {
				route = "unmatched"
			}
			logx.From(r.Context()).Info("request",
				"method", r.Method, "route", route, "status", sw.status, "ms", clock.Now().Sub(start).Milliseconds())
		})
	}
}

// BodyLimit caps request bodies: 64 KB, or 16 KB for signed server-to-server routes.
func BodyLimit(isSigned func(*http.Request) bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := int64(MaxBody)
			if isSigned(r) {
				limit = MaxWebhookBody
			}
			if r.ContentLength > limit {
				Error(w, r, apperr.New(apperr.PayloadTooLarge))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// Authn puts the caller in the context when authenticate recognises them. A nil
// authenticate (before sign-in exists) leaves every request anonymous.
func Authn(authenticate func(*http.Request) (authn.Actor, bool)) Middleware {
	return func(next http.Handler) http.Handler {
		if authenticate == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if actor, ok := authenticate(r); ok {
				r = r.WithContext(authn.With(r.Context(), actor))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// csrfHeader is the value every browser call must send (docs/06 section 1). A
// cross-site form or simple request cannot add a custom header.
const csrfHeaderValue = "gp-web"

// CORSAndCSRF allows only exact origins from ALLOWED_ORIGINS (never *, never
// "null", never reflected) and, for every state-changing request that is not
// signed, requires both an allowed Origin and X-Requested-With: gp-web.
func CORSAndCSRF(allowedOrigins []string, isSigned func(*http.Request) bool) Middleware {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			ok := origin != "" && allowed[origin]
			h := w.Header()
			h.Add("Vary", "Origin")
			if ok {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Expose-Headers", "X-Request-Id, ETag")
			}
			if r.Method == http.MethodOptions {
				if ok && r.Header.Get("Access-Control-Request-Method") != "" {
					h.Add("Vary", "Access-Control-Request-Method")
					h.Add("Vary", "Access-Control-Request-Headers")
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
					h.Set("Access-Control-Allow-Headers", "Content-Type, X-Requested-With, Idempotency-Key, If-None-Match")
					h.Set("Access-Control-Max-Age", "600")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if !isSafeMethod(r.Method) && !isSigned(r) &&
				!(ok && strings.TrimSpace(r.Header.Get("X-Requested-With")) == csrfHeaderValue) {
				Error(w, r, apperr.New(apperr.Forbidden))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets the headers every API response carries (docs/10 section
// 5). Cache-Control starts as no-store; only CachePublic loosens it.
func SecurityHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			h.Set("Cache-Control", "private, no-store")
			next.ServeHTTP(w, r)
		})
	}
}
