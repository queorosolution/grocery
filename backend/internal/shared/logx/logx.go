// Package logx is the one logger. It writes JSON lines and redacts personal
// data and secrets before anything reaches the log (DATA-01).
package logx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
)

const redacted = "[redacted]"

// sensitiveKeys are compared after lower-casing and dropping "_" and "-", so
// "Access_Token" and "accessToken" both match. The list is exact on purpose:
// a substring rule would also hide harmless keys such as "statusCode".
var sensitiveKeys = map[string]bool{
	"phone": true, "email": true, "token": true, "code": true, "secret": true,
	"password": true, "authorization": true, "cookie": true,
	"accesstoken": true, "refreshtoken": true, "apikey": true,
	"merchantsecret": true, "connectionstring": true, "otp": true,
}

var (
	// 9 to 15 digits, with optional spaces or dashes between them, with or without "+".
	phoneLike = regexp.MustCompile(`\+?\b\d(?:[ -]?\d){8,14}\b`)
	emailLike = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

func isSensitiveKey(key string) bool {
	key = strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
	return sensitiveKeys[key]
}

// RedactString masks phone numbers and e-mail addresses inside free text.
func RedactString(s string) string {
	return emailLike.ReplaceAllString(phoneLike.ReplaceAllString(s, "[phone]"), "[email]")
}

// New builds a JSON logger that redacts every attribute.
func New(level slog.Level, w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: redactAttr}))
}

func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, redacted)
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, RedactString(v.String()))
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, RedactString(err.Error()))
		}
		return slog.Any(a.Key, sanitizeAny(v.Any()))
	}
	return a
}

// sanitizeAny walks structs, maps and slices through their JSON form, so a
// logged struct with a Phone field is masked like a flat attribute.
func sanitizeAny(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return "[unloggable]"
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return "[unloggable]"
	}
	return walk(tree)
}

func walk(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSensitiveKey(k) {
				t[k] = redacted
			} else {
				t[k] = walk(val)
			}
		}
		return t
	case []any:
		for i := range t {
			t[i] = walk(t[i])
		}
		return t
	case string:
		return RedactString(t)
	}
	return v
}

// ParseLevel reads LOG_LEVEL.
func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	switch strings.ToLower(s) {
	case "debug":
		l = slog.LevelDebug
	case "info", "":
		l = slog.LevelInfo
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		return 0, fmt.Errorf("logx: unknown level %q", s)
	}
	return l, nil
}

var fallback atomic.Pointer[slog.Logger]

func init() { fallback.Store(New(slog.LevelInfo, os.Stdout)) }

// SetDefault sets the logger used when a context carries none. Call it once at start-up.
func SetDefault(l *slog.Logger) { fallback.Store(l) }

type ctxKey struct{}

// With returns a context that carries l, for example with a request id already attached.
func With(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// From returns the logger of the context, or the default one.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return fallback.Load()
}
