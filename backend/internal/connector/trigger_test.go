package connector_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gp/internal/connector"
	"gp/internal/shared/clock"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

func sign(secret string, ts time.Time, body string) (stamp, sig string) {
	stamp = strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "." + body))
	return stamp, "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// post sends the call a shop's own system would send: no cookies, only the signature.
func (r *rig) post(c connector.Connector, stamp, sig, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/"+c.ID.String()+"/trigger", strings.NewReader(body))
	req.Header.Set("X-GP-Timestamp", stamp)
	req.Header.Set("X-GP-Signature", sig)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.a.Handler.ServeHTTP(w, req)
	return w
}

func (r *rig) rotate(c connector.Connector) string {
	r.a.T.Helper()
	got := apptest.Decode[connector.TriggerSecret](r.a.T, r.admin.OK(http.MethodPost, r.connPath(c, "/trigger-secret"), ""))
	return got.Secret
}

func TestConnector_TriggerSecretIsShownOnce(t *testing.T) {
	r := newRig(t)
	c := r.mapped(r.reader)
	if apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodGet, r.path("/connector"), "")).HasTriggerSecret {
		t.Fatal("a new connector has a trigger secret")
	}
	w := r.admin.OK(http.MethodPost, r.connPath(c, "/trigger-secret"), "")
	got := apptest.Decode[connector.TriggerSecret](t, w)
	if !strings.HasPrefix(got.Secret, "gpt_") || len(got.Secret) < 40 || !strings.Contains(got.Example, c.ID.String()) {
		t.Fatalf("answer = %+v", got)
	}
	if strings.Contains(got.Example, got.Secret) {
		t.Error("the example holds the secret; it must name a variable")
	}
	if v, _ := r.a.Secrets.Get(t.Context(), "connectors/"+c.ID.String()+"/trigger"); v != got.Secret {
		t.Error("the secret is not under connectors/<id>/trigger")
	}
	later := r.admin.OK(http.MethodGet, r.path("/connector"), "").Body.String()
	runs := r.admin.OK(http.MethodGet, r.connPath(c, "/runs"), "").Body.String()
	for _, body := range []string{later, runs} {
		if strings.Contains(body, got.Secret) {
			t.Error("the secret came back in a later answer")
		}
	}
	if n := r.a.Count(`SELECT count(*) FROM audit_log WHERE before::text LIKE '%' || $1 || '%' OR after::text LIKE '%' || $1 || '%'`, got.Secret); n != 0 {
		t.Error("the secret is in the audit log")
	}
	if n := r.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'connector.trigger_secret.rotate'`); n != 1 {
		t.Errorf("rotation audit rows = %d, want 1", n)
	}
	// Rotating makes the old secret useless.
	next := r.rotate(c)
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")
	stamp, sig := sign(got.Secret, clock.Now(), "")
	if w := r.post(c, stamp, sig, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("old secret = %d, want 401", w.Code)
	}
	stamp, sig = sign(next, clock.Now(), "")
	if w := r.post(c, stamp, sig, ""); w.Code != http.StatusAccepted {
		t.Errorf("new secret = %d %s, want 202", w.Code, w.Body)
	}
}

// BIZ-16: the trigger URL needs a fresh time, a good HMAC and stays inside its hourly limit.
func TestConnector_TriggerIsSigned(t *testing.T) {
	r := newRig(t)
	c := r.mapped(r.reader)
	secret := r.rotate(c)
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")
	body := `{"externalIds":["A1","A2"]}`

	ok := func(stamp, sig, body string) { // a good call
		t.Helper()
		if w := r.post(c, stamp, sig, body); w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"queued":true`) {
			t.Fatalf("good call = %d %s", w.Code, w.Body)
		}
	}
	bad := func(what string, w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Errorf("%s = %d %s, want %d", what, w.Code, w.Body, code)
		}
	}

	stamp, sig := sign(secret, clock.Now(), body)
	ok(stamp, sig, body)
	if jobs := r.a.Invoker.Jobs(); len(jobs) != 1 || jobs[0].Trigger != enum.RunTriggerWebhook || len(jobs[0].ExternalIDs) != 2 {
		t.Errorf("jobs = %+v", jobs)
	}
	stamp, sig = sign(secret, clock.Now(), "")
	ok(stamp, sig, "") // no body: an incremental run

	old, oldSig := sign(secret, clock.Now().Add(-6*time.Minute), body)
	bad("a time six minutes old", r.post(c, old, oldSig, body), http.StatusUnauthorized)
	future, futureSig := sign(secret, clock.Now().Add(6*time.Minute), body)
	bad("a time six minutes ahead", r.post(c, future, futureSig, body), http.StatusUnauthorized)
	stamp, sig = sign("not-the-secret", clock.Now(), body)
	bad("a wrong secret", r.post(c, stamp, sig, body), http.StatusUnauthorized)
	stamp, sig = sign(secret, clock.Now(), body)
	bad("a changed body", r.post(c, stamp, sig, `{"externalIds":["A9"]}`), http.StatusUnauthorized)
	bad("no signature", r.post(c, stamp, "", body), http.StatusUnauthorized)
	bad("a signature that is not hex", r.post(c, stamp, "v1=zz", body), http.StatusUnauthorized)
	bad("no version", r.post(c, stamp, strings.TrimPrefix(sig, "v1="), body), http.StatusUnauthorized)
	bad("no time", r.post(c, "", sig, body), http.StatusUnauthorized)

	// A connector with no secret, or one that does not exist, answers like a bad signature.
	other := r.a.SeedShop("other")
	oc := apptest.Decode[connector.Connector](t, r.admin.OK(http.MethodPost, "/admin/shops/"+other.String()+"/connector",
		`{"connectionString":`+strconv.Quote(r.reader)+`}`))
	bad("a connector with no secret", r.post(oc, stamp, sig, body), http.StatusUnauthorized)

	// Past the signature: the body is checked, and a paused connector says so.
	tooMany := `{"externalIds":[` + strings.TrimSuffix(strings.Repeat(`"x",`, 101), ",") + `]}`
	stamp, sig = sign(secret, clock.Now(), tooMany)
	bad("101 ids", r.post(c, stamp, sig, tooMany), http.StatusBadRequest)
	stamp, sig = sign(secret, clock.Now(), `{"externalIds":[""]}`)
	bad("an empty id", r.post(c, stamp, sig, `{"externalIds":[""]}`), http.StatusBadRequest)
	stamp, sig = sign(secret, clock.Now(), `{"other":1}`)
	bad("an unknown field", r.post(c, stamp, sig, `{"other":1}`), http.StatusBadRequest)
	r.admin.OK(http.MethodPost, r.connPath(c, "/pause"), "")
	stamp, sig = sign(secret, clock.Now(), body)
	bad("a paused connector", r.post(c, stamp, sig, body), http.StatusConflict)
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")
}

func TestConnector_TriggerRateLimit(t *testing.T) {
	r := newRig(t)
	c := r.mapped(r.reader)
	secret := r.rotate(c)
	r.admin.OK(http.MethodPost, r.connPath(c, "/activate"), "")

	// Calls with a wrong signature do not use up the hour of the real shop.
	for i := 0; i < 40; i++ {
		stamp, sig := sign("guess", clock.Now(), "")
		if w := r.post(c, stamp, sig, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d = %d", i, w.Code)
		}
	}
	for i := 1; i <= 30; i++ {
		stamp, sig := sign(secret, clock.Now(), "")
		if w := r.post(c, stamp, sig, ""); w.Code != http.StatusAccepted {
			t.Fatalf("call %d = %d %s, want 202", i, w.Code, w.Body)
		}
	}
	stamp, sig := sign(secret, clock.Now(), "")
	w := r.post(c, stamp, sig, "")
	if w.Code != http.StatusTooManyRequests || apptest.ErrorOf(t, w).Error.Code != "RATE_LIMITED" {
		t.Errorf("call 31 = %d %s, want 429 RATE_LIMITED", w.Code, w.Body)
	}
}
