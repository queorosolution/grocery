package notify_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"gp/internal/notify"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/test/testdb"
)

const (
	customer  = "+94771234567"
	adminTel  = "+94770000000"
	someOrder = "KS-7Q2M9X"
)

// channel keeps what it was asked to send and can be told to fail.
type channel struct {
	mu   sync.Mutex
	sent []notify.Message
	fail error
}

func (c *channel) Send(_ context.Context, m notify.Message) (string, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return "", 0, c.fail
	}
	c.sent = append(c.sent, m)
	return "ref-1", 1, nil
}

func (c *channel) count(template string) (n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.sent {
		if m.Template == template {
			n++
		}
	}
	return n
}

type rig struct {
	t   *testing.T
	env *testdb.Env
	sms *channel
	wa  *channel
	svc *notify.Service
}

func newRig(t *testing.T, cap int) *rig {
	t.Helper()
	r := &rig{t: t, env: testdb.New(t), sms: &channel{}, wa: &channel{}}
	r.svc = notify.NewService(notify.Deps{DB: r.env.DB, SMS: r.sms, WALink: r.wa, DailyCap: cap, AdminPhone: adminTel, App: "NearShop"})
	return r
}

func (r *rig) count(where string, args ...any) (n int) {
	r.t.Helper()
	if err := r.env.Owner.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE `+where, args...).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func delivered(to, event string) notify.Message {
	return notify.Message{To: to, Template: notify.OrderDelivered, Vars: map[string]string{"app": "NearShop", "orderCode": someOrder}, EventID: event}
}

func TestService_SendChecksFirst(t *testing.T) {
	r := newRig(t, 100)
	ok := notify.Message{To: customer, Template: notify.OTP, Vars: map[string]string{"app": "P", "code": "1"}}
	if err := r.svc.Send(context.Background(), ok); err != nil || len(r.sms.sent) != 1 {
		t.Fatalf("send: %v, %d sent", err, len(r.sms.sent))
	}
	for name, m := range map[string]notify.Message{
		"no recipient":     {Template: notify.OTP, Vars: ok.Vars},
		"unknown template": {To: customer, Template: "x"},
		"missing var":      {To: customer, Template: notify.OTP},
	} {
		if err := r.svc.Send(context.Background(), m); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if len(r.sms.sent) != 1 {
		t.Errorf("a refused message reached the channel (%d sent)", len(r.sms.sent))
	}
}

// FR-NTF-03: every send is logged with the phone masked and no text.
func TestService_LogsEverySendMasked(t *testing.T) {
	r := newRig(t, 100)
	if err := r.svc.Send(context.Background(), delivered(customer, "evt_1")); err != nil {
		t.Fatal(err)
	}
	var to, status, tmpl, ref string
	var cost int
	err := r.env.Owner.QueryRow(context.Background(), `SELECT to_masked, status, template, provider_ref, cost_units FROM notifications`).Scan(&to, &status, &tmpl, &ref, &cost)
	if err != nil {
		t.Fatal(err)
	}
	if to == customer || strings.Contains(to, "771234") || status != "SENT" || tmpl != notify.OrderDelivered || ref != "ref-1" || cost != 1 {
		t.Errorf("row = %q %s %s %s %d", to, status, tmpl, ref, cost)
	}
	if r.sms.sent[0].Text == "" || !strings.Contains(r.sms.sent[0].Text, someOrder) {
		t.Errorf("the channel got no rendered text: %+v", r.sms.sent[0])
	}
}

// A retried event does not text anyone twice; a failed try may be repeated.
func TestService_SendsOncePerEvent(t *testing.T) {
	r := newRig(t, 100)
	ctx := context.Background()
	r.sms.fail = errors.New("sms: the provider answered 503")
	if err := r.svc.Send(ctx, delivered(customer, "evt_2")); err == nil {
		t.Fatal("a failed send must return the error so the event is retried")
	}
	if r.count(`status = 'FAILED' AND error LIKE '%503%'`) != 1 {
		t.Error("the failure is not logged")
	}
	r.sms.fail = nil
	for range 3 {
		if err := r.svc.Send(ctx, delivered(customer, "evt_2")); err != nil {
			t.Fatal(err)
		}
	}
	if r.sms.count(notify.OrderDelivered) != 1 {
		t.Errorf("sent %d times, want once", r.sms.count(notify.OrderDelivered))
	}
	// The same event for someone else, and another event for the same person, are separate texts.
	_ = r.svc.Send(ctx, delivered("+94779999999", "evt_2"))
	_ = r.svc.Send(ctx, delivered(customer, "evt_3"))
	if r.sms.count(notify.OrderDelivered) != 3 {
		t.Errorf("sent %d times, want 3", r.sms.count(notify.OrderDelivered))
	}
}

// FR-NTF-04: the daily cap skips the SMS, says so once, and warns at 80 %.
func TestService_DailyCap(t *testing.T) {
	r := newRig(t, 5)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if err := r.svc.Send(ctx, delivered(customer, "evt_a"+string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	if r.sms.count(notify.AdminCap80) != 0 {
		t.Fatal("the warning came before 80 %")
	}
	_ = r.svc.Send(ctx, delivered(customer, "evt_a4")) // 4 of 5 is 80 %
	_ = r.svc.Send(ctx, delivered(customer, "evt_a5")) // 5 of 5
	if r.sms.count(notify.AdminCap80) != 1 {
		t.Errorf("80 %% warnings = %d, want 1", r.sms.count(notify.AdminCap80))
	}
	if msg := lastTo(r.sms, notify.AdminCap80); msg != adminTel {
		t.Errorf("the warning went to %q", msg)
	}
	// The cap is reached: the text is skipped, and the admin hears of it once.
	for i := 6; i <= 8; i++ {
		err := r.svc.Send(ctx, delivered(customer, "evt_a"+string(rune('0'+i))))
		if !errors.Is(err, notify.ErrCapReached) {
			t.Fatalf("send %d: %v, want ErrCapReached", i, err)
		}
	}
	if r.sms.count(notify.OrderDelivered) != 5 || r.count(`status = 'SKIPPED' AND error = 'daily_cap'`) != 3 {
		t.Error("texts over the cap were sent, or the skips are not logged")
	}
	if r.sms.count(notify.AdminCap100) != 1 {
		t.Errorf("cap alerts = %d, want 1", r.sms.count(notify.AdminCap100))
	}
	// WhatsApp links cost nothing and are not stopped by the cap.
	m := delivered(customer, "evt_wa")
	m.Channel = enum.NotifyChannelWaLink
	if err := r.svc.Send(ctx, m); err != nil {
		t.Fatalf("link over the cap: %v", err)
	}
}

func lastTo(c *channel, template string) string {
	for i := len(c.sent) - 1; i >= 0; i-- {
		if c.sent[i].Template == template {
			return c.sent[i].To
		}
	}
	return ""
}

func TestService_Channels(t *testing.T) {
	r := newRig(t, 100)
	ctx := context.Background()
	m := delivered(customer, "evt_w")
	m.Channel = enum.NotifyChannelWaLink
	if err := r.svc.Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	if r.wa.count(notify.OrderDelivered) != 1 || r.count(`channel = 'WA_LINK' AND status = 'LINK'`) != 1 || len(r.sms.sent) != 0 {
		t.Error("a wa.me message must be a LINK row and not an SMS")
	}
	// WhatsApp through the Cloud API is phase 2: there is no channel, so it is logged as skipped.
	m = delivered(customer, "evt_x")
	m.Channel = enum.NotifyChannelWhatsapp
	if err := r.svc.Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	if r.count(`channel = 'WHATSAPP' AND status = 'SKIPPED' AND error = 'no_channel'`) != 1 {
		t.Error("a channel that does not exist must be logged as skipped")
	}
}

func TestService_UsageReport(t *testing.T) {
	r := newRig(t, 100)
	ctx := context.Background()
	_ = r.svc.Send(ctx, delivered(customer, "evt_u1"))
	_ = r.svc.Send(ctx, delivered(customer, "evt_u2"))
	r.sms.fail = errors.New("sms: down")
	_ = r.svc.Send(ctx, delivered(customer, "evt_u3"))
	u, err := r.svc.Usage(ctx, db.System(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, row := range u.Data {
		got[row.Channel+"/"+row.Status] += row.Count
	}
	if got["SMS/SENT"] != 2 || got["SMS/FAILED"] != 1 || u.SentToday != 2 || u.DailyCap != 100 {
		t.Errorf("usage = %+v", u)
	}
	for _, bad := range [][2]string{{"x", ""}, {"", "2026-13-01"}, {"2026-02-01", "2026-01-01"}, {"2020-01-01", "2026-01-01"}} {
		if _, err := r.svc.Usage(ctx, db.System(), bad[0], bad[1]); err == nil {
			t.Errorf("usage(%q, %q): want an error", bad[0], bad[1])
		}
	}
}
