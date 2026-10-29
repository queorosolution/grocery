package user_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"gp/internal/auth"
	"gp/internal/shared/logx"
	"gp/test/apptest"
	"gp/test/fakes"
)

const phone, e164 = "0771234567", "+94771234567"

func sendBody(p string) string {
	return `{"phone":"` + p + `","turnstileToken":"` + fakes.PassToken + `"}`
}

func verifyBody(code string) string { return `{"code":"` + code + `"}` }

func (f fixture) send(c apptest.Client, p string) int {
	return c.Call(http.MethodPost, "/me/phone/send", sendBody(p)).Code
}

func TestPhone_SendAndVerify(t *testing.T) {
	f := newFixture(t)
	if code := f.send(f.me, phone); code != http.StatusNoContent {
		t.Fatalf("send = %d, want 204", code)
	}
	sms := f.a.SMS.Sent()
	if len(sms) != 1 || sms[0].To != e164 || sms[0].Template != "otp" || sms[0].Vars["platform"] != "TestShop" {
		t.Fatalf("sms = %+v", sms)
	}
	code := f.a.SMS.LastCode(e164)
	if len(code) != 6 {
		t.Fatalf("code = %q, want 6 digits", code)
	}
	// Only a keyed hash is stored, never the code or anything that contains it.
	if n := f.a.Count(`SELECT count(*) FROM otp_codes WHERE position($1 in encode(code_hash, 'escape')) > 0 OR length(code_hash) <> 32`, code); n != 0 {
		t.Error("the code must be stored as a 32-byte hash only")
	}

	me := apptest.Decode[auth.Me](t, f.me.OK(http.MethodPost, "/me/phone/verify", verifyBody(code)))
	if !me.PhoneVerified || me.NeedsOnboarding || me.PhoneMasked == nil || *me.PhoneMasked != "077•••4567" {
		t.Fatalf("me after verify = %+v", me)
	}
	if n := f.a.Count(`SELECT count(*) FROM users WHERE phone = $1 AND phone_verified_at IS NOT NULL`, e164); n != 1 {
		t.Errorf("users with the verified phone = %d, want 1", n)
	}
	// The same code cannot be used twice.
	f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody(code), http.StatusBadRequest, "OTP_INVALID")
}

// AUTHN-09: five wrong tries, then the sixth is locked even with the right code.
func TestPhone_BruteForceLocks(t *testing.T) {
	f := newFixture(t)
	f.send(f.me, phone)
	code := f.a.SMS.LastCode(e164)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 1; i <= 5; i++ {
		f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody(wrong), http.StatusBadRequest, "OTP_INVALID")
	}
	f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody(wrong), http.StatusTooManyRequests, "OTP_LOCKED")
	f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody(code), http.StatusTooManyRequests, "OTP_LOCKED")
	if n := f.a.Count(`SELECT count(*) FROM users WHERE phone_verified_at IS NOT NULL`); n != 0 {
		t.Error("a locked code must never verify the phone")
	}
	// A new SMS gives a fresh code and a fresh counter.
	f.send(f.me, phone)
	fresh := f.a.SMS.LastCode(e164)
	f.me.OK(http.MethodPost, "/me/phone/verify", verifyBody(fresh))
}

func TestPhone_RefusedCodes(t *testing.T) {
	f := newFixture(t)
	f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody("123456"), http.StatusBadRequest, "OTP_INVALID") // nothing sent yet
	for _, bad := range []string{"", "12345a", "1234567", "123", " 1234", "12 34"} {
		f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody(bad), http.StatusBadRequest, "VALIDATION_ERROR")
	}

	f.send(f.me, phone)
	first := f.a.SMS.LastCode(e164)
	f.send(f.me, phone)
	second := f.a.SMS.LastCode(e164)
	if first != second {
		f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody(first), http.StatusBadRequest, "OTP_INVALID") // only the newest SMS works
	}

	f.a.Exec(`UPDATE otp_codes SET expires_at = now() - interval '1 second' WHERE used_at IS NULL`)
	f.me.Expect(http.MethodPost, "/me/phone/verify", verifyBody(second), http.StatusBadRequest, "OTP_INVALID")
}

// A code belongs to the user it was sent to.
func TestPhone_CodeIsNotTransferable(t *testing.T) {
	f := newFixture(t)
	other := f.another("kamal")
	f.send(f.me, phone)
	code := f.a.SMS.LastCode(e164)
	other.Expect(http.MethodPost, "/me/phone/verify", verifyBody(code), http.StatusBadRequest, "OTP_INVALID")
	if n := f.a.Count(`SELECT count(*) FROM users WHERE phone_verified_at IS NOT NULL`); n != 0 {
		t.Error("someone else's code verified a phone")
	}
}

// BIZ-13: SMS pumping.
func TestPhone_SendAbuseControls(t *testing.T) {
	f := newFixture(t)
	for name, p := range map[string]string{
		"foreign number": "+14155550123", "landline-looking": "0112345678", "too short": "077123", "letters": "07712abc67", "empty": "",
	} {
		w := f.me.Call(http.MethodPost, "/me/phone/send", sendBody(p))
		if w.Code != http.StatusBadRequest || apptest.ErrorOf(t, w).Error.Fields["phone"] == "" {
			t.Errorf("%s: %d %s, want 400 with a phone error", name, w.Code, w.Body)
		}
	}
	for _, token := range []string{"", "made-up", strings.Repeat("a", 3000)} {
		f.me.Expect(http.MethodPost, "/me/phone/send", `{"phone":"`+phone+`","turnstileToken":"`+token+`"}`, http.StatusBadRequest, "BOT_CHECK_FAILED")
	}
	if len(f.a.SMS.Sent()) != 0 {
		t.Fatal("a refused request must send no SMS")
	}

	// 3 per hour per user: the 4th is 429.
	for i := 1; i <= 3; i++ {
		if code := f.send(f.me, phone); code != http.StatusNoContent {
			t.Fatalf("send %d = %d", i, code)
		}
	}
	f.me.Expect(http.MethodPost, "/me/phone/send", sendBody(phone), http.StatusTooManyRequests, "RATE_LIMITED")
	if got := len(f.a.SMS.Sent()); got != 3 {
		t.Errorf("%d SMS sent, want 3", got)
	}
}

// 5 per day per phone, however many accounts ask.
func TestPhone_PerNumberDailyCap(t *testing.T) {
	f := newFixture(t)
	other := f.another("kamal")
	third := f.another("sunil")
	for i := 0; i < 3; i++ {
		f.send(f.me, phone)
	}
	for i := 0; i < 2; i++ {
		f.send(other, phone)
	}
	third.Expect(http.MethodPost, "/me/phone/send", sendBody("+94 77 123 4567"), http.StatusTooManyRequests, "RATE_LIMITED") // same number, other spelling
	if third.Call(http.MethodPost, "/me/phone/send", sendBody("0779999999")).Code != http.StatusNoContent {
		t.Error("another number must still work")
	}
	if got := len(f.a.SMS.Sent()); got != 6 {
		t.Errorf("%d SMS sent, want 6", got)
	}
}

// DATA-01: neither the code nor the number reaches the log, even on a wrong try.
func TestPhone_NothingSecretIsLogged(t *testing.T) {
	var logs bytes.Buffer
	logx.SetDefault(logx.New(slog.LevelDebug, &logs))
	t.Cleanup(func() { logx.SetDefault(logx.New(slog.LevelInfo, os.Stdout)) })

	f := newFixture(t)
	f.send(f.me, phone)
	code := f.a.SMS.LastCode(e164)
	f.me.Call(http.MethodPost, "/me/phone/verify", verifyBody("000000"))
	f.me.Call(http.MethodPost, "/me/phone/verify", verifyBody(code))
	out := logs.String()
	if out == "" {
		t.Fatal("expected some log output")
	}
	for _, secret := range []string{code, "771234567", phone, "turnstile-pass"} {
		if strings.Contains(out, secret) {
			t.Errorf("log contains %q", secret)
		}
	}
}
