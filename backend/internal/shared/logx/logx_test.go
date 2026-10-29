package logx

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func logged(t *testing.T, fn func(l *slog.Logger)) string {
	t.Helper()
	var buf bytes.Buffer
	fn(New(slog.LevelDebug, &buf))
	return buf.String()
}

func mustNotContain(t *testing.T, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("log leaks %q:\n%s", s, out)
		}
	}
}

// DATA-01: log a struct with phone and token, the output is masked.
func TestRedaction(t *testing.T) {
	type customer struct {
		Name   string `json:"name"`
		Phone  string `json:"phone"`
		Token  string `json:"token"`
		Nested struct {
			Email string `json:"email"`
			Note  string `json:"note"`
		} `json:"nested"`
	}
	c := customer{Name: "Nimal", Phone: "+94771234567", Token: "abc.def.ghi"}
	c.Nested.Email = "nimal@example.com"
	c.Nested.Note = "call 077 123 4567 after 5"

	out := logged(t, func(l *slog.Logger) {
		l.Info("order from 0771234567 submitted",
			"customer", c,
			"phone", "+94771234567",
			"Access_Token", "tok123",
			"password", "hunter2",
			"cookie", "gp_at=zzz",
			"authorization", "Bearer yyy",
			"secret", "s3cr3t",
			"code", "482913",
			"err", errors.New("sms to +94 77 123 4567 failed"),
			slog.Group("req", slog.String("email", "a@b.lk"), slog.String("path", "/orders")),
			"list", []string{"ring 0112345678"},
			"orderId", "KS-7Q2M9X", "statusCode", 502,
		)
	})
	mustNotContain(t, out, "+94771234567", "0771234567", "077 123 4567", "tok123", "hunter2", "gp_at=zzz",
		"Bearer yyy", "s3cr3t", "482913", "nimal@example.com", "abc.def.ghi", "a@b.lk", "0112345678", "77 123 4567")
	for _, keep := range []string{"Nimal", "KS-7Q2M9X", "statusCode", "502", "/orders", "[redacted]", "[phone]"} {
		if !strings.Contains(out, keep) {
			t.Errorf("log lost %q:\n%s", keep, out)
		}
	}
}

func TestOrdinaryValuesSurvive(t *testing.T) {
	out := logged(t, func(l *slog.Logger) {
		l.Info("paid", "amountCents", 143500, "orderId", "KS-7Q2M9X", "id", "3f2a9c1e-5b7d-4e8a-9c21-7d4e5f6a8b90", "evt", "evt_01JABCDEF")
	})
	mustNotContain(t, out, "[phone]", "[redacted]")
}

func TestRedactStringPhoneShapes(t *testing.T) {
	for _, in := range []string{"0771234567", "+94771234567", "94 77 123 4567", "077-123-4567"} {
		if got := RedactString("x " + in + " y"); got != "x [phone] y" {
			t.Errorf("RedactString(%q) = %q", in, got)
		}
	}
	for _, in := range []string{"KS-7Q2M9X", "143500", "12345678", "v1.2.3"} {
		if got := RedactString(in); got != in {
			t.Errorf("RedactString(%q) changed to %q", in, got)
		}
	}
}

func TestFromReturnsContextLogger(t *testing.T) {
	var buf bytes.Buffer
	ctx := With(context.Background(), New(slog.LevelInfo, &buf).With("requestId", "r1"))
	From(ctx).Info("hello")
	if !strings.Contains(buf.String(), "r1") {
		t.Fatalf("context logger not used: %s", buf.String())
	}
	if From(context.Background()) == nil {
		t.Fatal("From must never return nil")
	}
}

func TestParseLevel(t *testing.T) {
	if l, err := ParseLevel("WARN"); err != nil || l != slog.LevelWarn {
		t.Fatalf("got %v, %v", l, err)
	}
	if l, err := ParseLevel(""); err != nil || l != slog.LevelInfo {
		t.Fatalf("empty must default to info, got %v, %v", l, err)
	}
	if _, err := ParseLevel("loud"); err == nil {
		t.Fatal("unknown level accepted")
	}
}

func TestRedactSecrets(t *testing.T) {
	in := map[string]any{
		"phone": "+94771234567", "email": "a@b.test", "merchantSecret": "hunter2",
		"nested": []any{map[string]any{"token": "abc", "name": "Krishna"}},
	}
	got := RedactSecrets(in).(map[string]any)
	if got["merchantSecret"] != "[redacted]" || got["phone"] != "+94771234567" || got["email"] != "a@b.test" {
		t.Fatalf("got %v", got)
	}
	nested := got["nested"].([]any)[0].(map[string]any)
	if nested["token"] != "[redacted]" || nested["name"] != "Krishna" {
		t.Fatalf("nested %v", nested)
	}
}
