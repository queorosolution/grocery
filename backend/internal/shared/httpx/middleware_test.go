package httpx

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/shared/authn"
	"gp/internal/shared/logx"
)

const (
	okOrigin = "https://www.nearshop.lk"
	evil     = "https://evil.com"
)

var noSig = func(*http.Request) bool { return false }

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func do(h http.Handler, method, origin string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/v1/x", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func cors() http.Handler {
	return CORSAndCSRF([]string{okOrigin, "https://dev.nearshop.lk"}, noSig)(ok())
}

// CSRF-01: another site posts with the user's cookies.
func TestCSRF01(t *testing.T) {
	h := cors()
	good := map[string]string{"X-Requested-With": "gp-web"}
	cases := []struct {
		name    string
		method  string
		origin  string
		headers map[string]string
		want    int
	}{
		{"curl POST without header or origin", "POST", "", nil, 403},
		{"POST with header but no origin", "POST", "", good, 403},
		{"POST with origin but no header", "POST", okOrigin, nil, 403},
		{"POST from evil origin with header", "POST", evil, good, 403},
		{"POST with wrong header value", "POST", okOrigin, map[string]string{"X-Requested-With": "XMLHttpRequest"}, 403},
		{"POST from allowed origin with header", "POST", okOrigin, good, 200},
		{"PATCH from allowed origin with header", "PATCH", okOrigin, good, 200},
		{"DELETE without header", "DELETE", okOrigin, nil, 403},
		{"PUT from evil", "PUT", evil, good, 403},
		{"GET needs neither", "GET", "", nil, 200},
		{"GET from evil still reaches the handler, but gets no CORS headers", "GET", evil, nil, 200},
	}
	for _, tc := range cases {
		if got := do(h, tc.method, tc.origin, tc.headers).Code; got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

// CORS-01: no wildcard or reflected origin with credentials.
func TestCORS01(t *testing.T) {
	h := cors()
	w := do(h, "GET", evil, nil)
	if w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("evil origin got CORS headers: %v", w.Header())
	}
	w = do(h, "GET", okOrigin, nil)
	if w.Header().Get("Access-Control-Allow-Origin") != okOrigin || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed origin missing CORS headers: %v", w.Header())
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatal("wildcard origin")
	}
	for _, name := range []string{"GET", "POST"} {
		if !strings.Contains(strings.Join(do(h, name, evil, map[string]string{"X-Requested-With": "gp-web"}).Header().Values("Vary"), ","), "Origin") {
			t.Errorf("%s: Vary: Origin missing, a shared cache could serve one origin's headers to another", name)
		}
	}
}

// CORS-02: the "null" origin is never accepted.
func TestCORS02(t *testing.T) {
	h := cors()
	if w := do(h, "GET", "null", nil); w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("null origin allowed: %v", w.Header())
	}
	if w := do(h, "POST", "null", map[string]string{"X-Requested-With": "gp-web"}); w.Code != 403 {
		t.Fatalf("POST from null origin got %d", w.Code)
	}
}

func TestOriginMatchIsExact(t *testing.T) {
	h := cors()
	for _, o := range []string{
		"https://www.nearshop.lk/", "https://WWW.nearshop.lk", "http://www.nearshop.lk",
		"https://www.nearshop.lk.evil.com", "https://evil.com?https://www.nearshop.lk", "https://sub.www.nearshop.lk",
	} {
		if w := do(h, "GET", o, nil); w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("origin %q was allowed", o)
		}
	}
}

func TestPreflight(t *testing.T) {
	h := cors()
	pre := map[string]string{"Access-Control-Request-Method": "POST", "Access-Control-Request-Headers": "x-requested-with"}
	w := do(h, "OPTIONS", okOrigin, pre)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != okOrigin ||
		!strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-Requested-With") ||
		!strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatalf("preflight from an allowed origin: %d %v", w.Code, w.Header())
	}
	w = do(h, "OPTIONS", evil, pre)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "" || w.Header().Get("Access-Control-Allow-Methods") != "" {
		t.Fatalf("preflight from evil must get no CORS headers: %d %v", w.Code, w.Header())
	}
}

func TestSignedRoutesSkipCSRF(t *testing.T) {
	signed := func(r *http.Request) bool { return r.URL.Path == "/v1/webhooks/payhere" }
	h := CORSAndCSRF([]string{okOrigin}, signed)(ok())
	r := httptest.NewRequest("POST", "/v1/webhooks/payhere", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("webhook got %d: payment providers cannot send an Origin or our header", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/other", nil))
	if w.Code != 403 {
		t.Fatalf("a non-signed POST got %d", w.Code)
	}
}

func TestRequestIDIsFreshAndOnTheLogger(t *testing.T) {
	var logs bytes.Buffer
	ctx := logx.With(context.Background(), logx.New(slog.LevelDebug, &logs))
	var seen string
	h := WithRequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
		logx.From(r.Context()).Info("hello")
	}))
	r := httptest.NewRequest("GET", "/x", nil).WithContext(ctx)
	r.Header.Set("X-Request-Id", "client-chosen")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.HasPrefix(seen, "req_") || w.Header().Get("X-Request-Id") != seen || seen == "client-chosen" {
		t.Fatalf("request id %q header %q", seen, w.Header().Get("X-Request-Id"))
	}
	if !strings.Contains(logs.String(), seen) {
		t.Fatalf("request id missing from log lines: %s", logs.String())
	}
}

func TestRecoverReturnsGeneric500AndLogsTheStack(t *testing.T) {
	var logs bytes.Buffer
	ctx := logx.With(context.Background(), logx.New(slog.LevelDebug, &logs))
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("db password is hunter2") }),
		WithRequestID(), Recover())
	r := httptest.NewRequest("GET", "/x", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 500 || strings.Contains(w.Body.String(), "hunter2") || !strings.Contains(w.Body.String(), "INTERNAL") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(logs.String(), "hunter2") || !strings.Contains(logs.String(), "stack") {
		t.Fatalf("panic value and stack must be logged: %s", logs.String())
	}
}

func TestAccessLogHasRouteNotPathOrQuery(t *testing.T) {
	var logs bytes.Buffer
	ctx := logx.With(context.Background(), logx.New(slog.LevelDebug, &logs))
	h := AccessLog()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetRoute(r.Context(), "GET /v1/orders/{id}")
		w.WriteHeader(http.StatusTeapot)
	}))
	r := httptest.NewRequest("GET", "/v1/orders/3f2a9c1e?phone=0771234567&token=abc", nil).WithContext(ctx)
	h.ServeHTTP(httptest.NewRecorder(), r)
	out := logs.String()
	for _, want := range []string{"GET /v1/orders/{id}", `"status":418`, `"method":"GET"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log misses %q: %s", want, out)
		}
	}
	for _, leak := range []string{"3f2a9c1e", "0771234567", "token=abc"} {
		if strings.Contains(out, leak) {
			t.Errorf("log leaks %q: %s", leak, out)
		}
	}
}

func TestBodyLimit(t *testing.T) {
	read := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1<<20)
		if _, err := r.Body.Read(buf); err != nil && err.Error() != "EOF" {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	signed := func(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/v1/webhooks") }
	h := BodyLimit(signed)(read)
	cases := []struct {
		path string
		size int
		want int
	}{
		{"/v1/x", MaxBody, 200},
		{"/v1/x", MaxBody + 1, 413},
		{"/v1/webhooks/payhere", MaxWebhookBody, 200},
		{"/v1/webhooks/payhere", MaxWebhookBody + 1, 413},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("POST", tc.path, strings.NewReader(strings.Repeat("a", tc.size)))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s with %d bytes: got %d, want %d", tc.path, tc.size, w.Code, tc.want)
		}
	}
}

func TestBodyLimitRejectsDeclaredOversizeEarly(t *testing.T) {
	called := false
	h := BodyLimit(noSig)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := httptest.NewRequest("POST", "/x", strings.NewReader("x"))
	r.ContentLength = MaxBody + 1
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 413 || called || !strings.Contains(w.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("got %d %s called=%v", w.Code, w.Body.String(), called)
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	SecurityHeaders()(ok()).ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	for k, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Cache-Control":             "private, no-store",
	} {
		if got := w.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestChainOrderFirstIsOutermost(t *testing.T) {
	var order []string
	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") }),
		mark("a"), mark("b"), mark("c")).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if strings.Join(order, ",") != "a,b,c,handler" {
		t.Fatalf("order %v", order)
	}
}

func TestAuthn(t *testing.T) {
	id := uuid.New()
	var got authn.Actor
	var found bool
	h := Authn(func(r *http.Request) (authn.Actor, bool) {
		return authn.Actor{UserID: id}, r.Header.Get("X-Test") == "yes"
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got, found = authn.From(r.Context()) }))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if found {
		t.Fatal("anonymous request got an actor")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Test", "yes")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !found || got.UserID != id {
		t.Fatal("actor not in context")
	}
	if Authn(nil)(ok()) == nil {
		t.Fatal("nil authenticator must keep requests anonymous")
	}
}
