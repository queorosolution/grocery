package order_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/notify"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

const deskPhone = "+94112345678"

// textsOf drains the events and returns what was sent, as (recipient, template, text) lines.
func (f fixture) texts() []notify.Message {
	f.a.T.Helper()
	if err := f.a.DrainEvents(); err != nil {
		f.a.T.Fatal(err)
	}
	return f.a.SMS.Sent()
}

func (f fixture) phoneOf(email string) string {
	f.a.T.Helper()
	var p string
	if err := f.a.Env.Owner.QueryRow(context.Background(), `SELECT phone FROM users WHERE email = $1`, email).Scan(&p); err != nil {
		f.a.T.Fatal(err)
	}
	return p
}

func find(msgs []notify.Message, template, to string) *notify.Message {
	for i := range msgs {
		if msgs[i].Template == template && msgs[i].To == to {
			return &msgs[i]
		}
	}
	return nil
}

func (f fixture) useDesk() {
	f.a.Exec(`UPDATE shops SET phone = $2 WHERE id = $1`, f.shop, deskPhone)
}

// FR-NTF-02: the texts of the whole flow go to the right person, with the right values.
func TestNotify_OrderFlow(t *testing.T) {
	f := newFixture(t)
	f.useDesk()
	s := f.paidFlow() // submitted, approved, paid
	customer := f.phoneOf("nimal@example.com")
	f.move(s.staff, s.id, "PICKING")
	f.move(s.staff, s.id, "PACKED")
	f.assign(s.staff, f.consolePath(f.shop, "/"+s.id.String()+"/driver"), s.id, s.driverID)
	f.move(s.staff, s.id, "OUT_FOR_DELIVERY")
	code := f.code(s.id)
	s.driver.OK(http.MethodPost, driverPath(s.id, "/complete"), version(f.get(f.me, s.id).Version, `"code":"`+code+`"`))
	msgs := f.texts()

	orderCode := f.get(f.me, s.id).Code
	if m := find(msgs, notify.ShopNewBill, deskPhone); m == nil || m.Vars["items"] != "2" || m.Vars["orderCode"] != orderCode {
		t.Errorf("new bill to the shop: %+v", m)
	}
	if m := find(msgs, notify.OrderApproved, customer); m == nil || !strings.HasSuffix(m.Text, "/order/"+s.id.String()) || !strings.Contains(m.Text, "https://www.nearshop.test/") {
		t.Errorf("approved to the customer: %+v", m)
	}
	if m := find(msgs, notify.OrderPaid, deskPhone); m == nil || strings.Contains(m.Text, "courier") || !strings.Contains(m.Text, "Slot ") {
		t.Errorf("paid to the shop: %+v", m)
	}
	if m := find(msgs, notify.OrderDelivered, customer); m == nil {
		t.Error("delivered to the customer: none")
	}
	// The code is sent for the order that was out for delivery, and by the time the worker
	// runs it the order is already delivered: a stale code is not sent.
	if find(msgs, notify.OrderOut, customer) != nil {
		t.Error("a code was sent for an order that was no longer out for delivery")
	}
}

// FR-DLV-03: the code goes to the customer by SMS, and to nobody else.
func TestNotify_DeliveryCodeOnlyToTheCustomer(t *testing.T) {
	f := newFixture(t)
	f.useDesk()
	s := f.outForDelivery()
	code := f.code(s.id)
	customer := f.phoneOf("nimal@example.com")
	msgs := f.texts()
	out := find(msgs, notify.OrderOut, customer)
	if out == nil || !strings.Contains(out.Text, "code "+code+" ") {
		t.Fatalf("out-for-delivery text = %+v", out)
	}
	for _, m := range msgs {
		if strings.Contains(m.Text, "code "+code) && m.To != customer {
			t.Errorf("the code went to %s in %s", m.To, m.Template)
		}
		if strings.Contains(strings.ToLower(m.Template), "driver") && m.To != customer {
			t.Errorf("unexpected text %s", m.Template)
		}
	}
}

func TestNotify_RunningTwiceSendsOnce(t *testing.T) {
	f := newFixture(t)
	f.useDesk()
	f.trio()
	first := len(f.texts())
	if first == 0 {
		t.Fatal("no text for a new bill")
	}
	// A second delivery of the same events (the queue sends twice) adds nothing.
	f.a.Exec(`DELETE FROM processed_events`)
	if second := len(f.texts()); second != first {
		t.Errorf("sent %d texts after the repeat, want %d", second, first)
	}
}

// FR-NTF-01: the shop's channels decide how its texts go; none means none.
func TestNotify_ShopChannels(t *testing.T) {
	cases := map[string]struct {
		channels string
		sms      int
		links    int
	}{
		"SMS":       {`{SMS}`, 1, 0},
		"WA_LINK":   {`{WA_LINK}`, 0, 1},
		"both":      {`{SMS,WA_LINK}`, 1, 1},
		"none":      {`{}`, 0, 0},
		"WHATSAPP":  {`{WHATSAPP}`, 0, 0}, // the Cloud API is phase 2
		"all three": {`{SMS,WHATSAPP,WA_LINK}`, 1, 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.useDesk()
			f.a.Exec(`UPDATE shop_settings SET notify_channels = $2::text[] WHERE shop_id = $1`, f.shop, c.channels)
			f.trio()
			msgs := f.texts()
			n := 0
			for _, m := range msgs {
				if m.Template == notify.ShopNewBill {
					n++
				}
			}
			if n != c.sms {
				t.Errorf("sms = %d, want %d", n, c.sms)
			}
			if got := f.a.Count(`SELECT count(*) FROM notifications WHERE template = 'shop.new_bill' AND channel = 'WA_LINK' AND status = 'LINK'`); got != c.links {
				t.Errorf("links = %d, want %d", got, c.links)
			}
		})
	}
}

// HDR-01: a shop name with new lines and braces cannot change the text.
func TestNotify_HDR01_ShopNameIsCleaned(t *testing.T) {
	f := newFixture(t)
	f.a.Exec(`UPDATE shops SET name = $2 WHERE id = $1`, f.shop, "Krishna\r\nStores {code} https://evil.lk/x")
	x := f.trio()
	f.review(f.staff("asha", "SHOP_STAFF"), x.o.ID, version(1, `"action":"APPROVE"`))
	m := find(f.texts(), notify.OrderApproved, f.phoneOf("nimal@example.com"))
	if m == nil {
		t.Fatal("no approved text")
	}
	if strings.ContainsAny(m.Text, "\r\n") || strings.Contains(m.Text, "evil.lk/x") || strings.Contains(m.Text, "{") || len([]rune(m.Text)) > 160 {
		t.Errorf("text = %q", m.Text)
	}
}

func TestNotify_AdminAlerts(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	f.fullRequest(s.staff, s.id) // refund.requested
	err := f.a.Env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		_, err := events.Add(ctx, tx, events.Event{Type: enum.EventTypeConnectorFailed, ShopID: f.shop, EntityID: uuid.NewString(), Payload: map[string]any{"failures": 3}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs := f.texts()
	if m := find(msgs, notify.AdminRefund, apptest.AdminPhone); m == nil || !strings.Contains(m.Text, "Rs ") || m.Vars["amount"] == "" {
		t.Errorf("refund alert: %+v", m)
	}
	if m := find(msgs, notify.AdminConnector, apptest.AdminPhone); m == nil || m.Vars["shop"] == "" {
		t.Errorf("connector alert: %+v", m)
	}
}

func TestNotify_CancelAndExpire(t *testing.T) {
	f := newFixture(t)
	f.useDesk()
	x := f.trio()
	customer := f.phoneOf("nimal@example.com")
	f.me.OK(http.MethodPost, "/orders/"+x.o.ID.String()+"/cancel", `{"version":1}`)
	msgs := f.texts()
	if find(msgs, notify.OrderCancelled, deskPhone) == nil {
		t.Error("the shop is not told of a cancel")
	}
	if find(msgs, notify.OrderCancelled, customer) != nil {
		t.Error("the customer who cancelled is told of it")
	}
	// Expired: both hear of it.
	g := newFixture(t)
	g.useDesk()
	y := g.trio()
	g.expire(y.o.ID, events.TimerReview, 0)
	m2 := g.texts()
	if find(m2, notify.OrderExpired, g.phoneOf("nimal@example.com")) == nil || find(m2, notify.OrderExpired, deskPhone) == nil {
		t.Error("an expired order must be told to the customer and the shop")
	}
	// Admin cancels a paid order (T19): the customer and the shop hear of the refund.
	h := newFixture(t)
	h.useDesk()
	s := h.paidFlow()
	h.admin().OK(http.MethodPost, "/admin/orders/"+s.id.String()+"/transition", version(h.get(h.me, s.id).Version, `"to":"CANCELLED_REFUND","reason":"Shop closed"`))
	m3 := h.texts()
	if find(m3, notify.OrderRefundCancel, h.phoneOf("nimal@example.com")) == nil || find(m3, notify.OrderRefundCancel, deskPhone) == nil {
		t.Error("a cancelled order with a refund must be told to the customer and the shop")
	}
}

// The day's cap stops events without failing them, and tells the person waiting for an OTP.
func TestNotify_CapStopsTextsNotEvents(t *testing.T) {
	f := newFixture(t)
	f.useDesk()
	f.a.Exec(`INSERT INTO notifications (channel, template, to_masked, status) SELECT 'SMS', 'order.expired', '+9477***0000', 'SENT' FROM generate_series(1, $1)`, apptest.SMSCap)
	f.trio()
	if err := f.a.DrainEvents(); err != nil {
		t.Fatalf("a text over the cap must not fail the event: %v", err)
	}
	if n := len(f.a.SMS.Sent()); n != 1 { // only the cap alert reaches the admin
		t.Errorf("%d texts sent over the cap, want just the alert", n)
	}
	if f.a.Count(`SELECT count(*) FROM notifications WHERE status = 'SKIPPED' AND error = 'daily_cap'`) == 0 {
		t.Error("the skipped text is not logged")
	}
	// A person waiting for an OTP is told to try later, not left waiting.
	me := f.a.As(f.a.SignedIn(claims("cap-user")))
	me.Expect(http.MethodPost, "/me/phone/send", `{"phone":"0771112222","turnstileToken":"`+fakes.PassToken+`"}`, http.StatusTooManyRequests, "RATE_LIMITED")
}
