// Package consumer turns the events of docs/04 section 5 into texts. Every rule
// is one line of the table below: which event, who hears of it, which template.
// The consumers run in the worker, in the system scope, and are safe to run twice:
// notify.Service sends a template to a recipient once per event.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/notify"
	"gp/internal/order"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/logx"
	"gp/internal/shared/money"
	"gp/internal/shop"
	"gp/internal/user"
)

// What the consumers read, as services of the other modules (never their repos).
type (
	Orders interface {
		NotifyFacts(ctx context.Context, tx *db.Tx, id uuid.UUID) (order.NotifyFacts, error)
		DeliveryCodeOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (string, error)
	}
	Shops interface {
		NotifyTarget(ctx context.Context, tx *db.Tx, id uuid.UUID) (shop.NotifyTarget, error)
	}
	Users interface {
		ContactOf(ctx context.Context, tx *db.Tx, userID uuid.UUID) (user.Contact, error)
	}
)

type who int

const (
	customer who = iota
	desk         // the shop's phone, on the shop's channels
	admin        // ADMIN_ALERT_PHONE, always by SMS
)

type rule struct {
	to       who
	template string
}

// rules is docs/04 section 5, "Consumers in worker", for the notify column.
var rules = map[enum.EventType][]rule{
	enum.EventTypeOrderSubmitted:       {{desk, notify.ShopNewBill}},
	enum.EventTypeOrderApproved:        {{customer, notify.OrderApproved}},
	enum.EventTypeOrderRevised:         {{customer, notify.OrderRevised}},
	enum.EventTypeOrderRejected:        {{customer, notify.OrderRejected}},
	enum.EventTypeOrderExpired:         {{customer, notify.OrderExpired}, {desk, notify.OrderExpired}},
	enum.EventTypeOrderCancelled:       {{desk, notify.OrderCancelled}},
	enum.EventTypeOrderPaid:            {{desk, notify.OrderPaid}},
	enum.EventTypeOrderOutForDelivery:  {{customer, notify.OrderOut}},
	enum.EventTypeOrderDelivered:       {{customer, notify.OrderDelivered}},
	enum.EventTypeOrderDeliveryFailed:  {{desk, notify.OrderDeliveryFailed}},
	enum.EventTypeOrderCancelledRefund: {{customer, notify.OrderRefundCancel}, {desk, notify.OrderRefundCancel}},
	enum.EventTypeRefundRequested:      {{admin, notify.AdminRefund}},
	enum.EventTypeConnectorFailed:      {{admin, notify.AdminConnector}},
}

// Config is the platform's own values that go into texts.
type Config struct {
	App, Domain, AdminPhone string
}

type Notifier struct {
	sender notify.Sender
	orders Orders
	shops  Shops
	users  Users
	cfg    Config
}

func New(s notify.Sender, o Orders, sh Shops, u Users, cfg Config) *Notifier {
	return &Notifier{sender: s, orders: o, shops: sh, users: u, cfg: cfg}
}

// Register adds one consumer per event in the table.
func (n *Notifier) Register(reg *events.Registry) {
	for typ := range rules {
		reg.On(typ, events.Consumer{Name: "notify." + string(typ), Handle: n.handle})
	}
}

// eventData is the part of an event's data that the texts need.
type eventData struct {
	OrderID     uuid.UUID   `json:"orderId"`
	AmountCents money.Cents `json:"amountCents"`
}

func (n *Notifier) handle(ctx context.Context, tx *db.Tx, m events.Message) error {
	var d eventData
	if err := json.Unmarshal(m.Data, &d); err != nil {
		return fmt.Errorf("notify: bad event data: %w", err)
	}
	var shopID uuid.UUID
	if m.ShopID != nil {
		shopID = *m.ShopID
	}
	ev := event{m: m, d: d}
	if d.OrderID != uuid.Nil {
		f, err := n.orders.NotifyFacts(ctx, tx, d.OrderID)
		if err != nil {
			return err
		}
		ev.order = &f
		shopID = f.ShopID
	}
	target, err := n.shops.NotifyTarget(ctx, tx, shopID)
	if err != nil {
		return err
	}
	ev.shop = target
	ev.shopID = shopID
	for _, r := range rules[m.Type] {
		if err := n.deliver(ctx, tx, ev, r); err != nil {
			return err
		}
	}
	return nil
}

type event struct {
	m      events.Message
	d      eventData
	order  *order.NotifyFacts
	shop   shop.NotifyTarget
	shopID uuid.UUID
}

// deliver sends one rule's text on every channel the recipient takes. A cap that
// is reached skips the text and goes on: waiting would only repeat the refusal.
func (n *Notifier) deliver(ctx context.Context, tx *db.Tx, ev event, r rule) error {
	phone, userID, channels, err := n.recipient(ctx, tx, ev, r.to)
	if err != nil || phone == "" {
		if err == nil {
			logx.From(ctx).Warn("notify.no_phone", "event", ev.m.Type, "template", r.template)
		}
		return err
	}
	vars, err := n.vars(ctx, tx, ev, r.template)
	if errors.Is(err, errStale) {
		return nil // the order moved on; a text about the old state would mislead
	} else if err != nil {
		return err
	}
	for _, ch := range channels {
		msg := notify.Message{To: phone, Template: r.template, Vars: vars, Channel: ch, UserID: userID, EventID: ev.m.ID}
		if ev.shopID != uuid.Nil {
			msg.ShopID = &ev.shopID
		}
		if err := n.sender.Send(ctx, msg); err != nil && !errors.Is(err, notify.ErrCapReached) {
			return err
		}
	}
	return nil
}

// recipient says where a rule's text goes. Customers and the desk follow the
// shop's channels (FR-NTF-01); WhatsApp through the Cloud API is phase 2, so that
// flag sends nothing yet. The admin always gets an SMS.
func (n *Notifier) recipient(ctx context.Context, tx *db.Tx, ev event, to who) (phone string, userID *uuid.UUID, channels []enum.NotifyChannel, err error) {
	sms := []enum.NotifyChannel{enum.NotifyChannelSms}
	switch to {
	case admin:
		return n.cfg.AdminPhone, nil, sms, nil
	case desk:
		return ev.shop.Phone, nil, usable(ev.shop.Channels), nil
	}
	if ev.order == nil {
		return "", nil, nil, nil
	}
	c, err := n.users.ContactOf(ctx, tx, ev.order.CustomerID)
	return c.Phone, &ev.order.CustomerID, usable(ev.shop.Channels), err
}

// usable keeps the channels that exist today.
func usable(chosen []enum.NotifyChannel) []enum.NotifyChannel {
	var out []enum.NotifyChannel
	for _, c := range chosen {
		if c == enum.NotifyChannelSms || c == enum.NotifyChannelWaLink {
			out = append(out, c)
		}
	}
	return out
}

// errStale says the text no longer fits the order.
var errStale = errors.New("notify: the order moved on")

// vars gathers the values a template may use. The delivery code is read only for
// the one text that holds it.
func (n *Notifier) vars(ctx context.Context, tx *db.Tx, ev event, template string) (map[string]string, error) {
	v := map[string]string{"app": n.cfg.App, "domain": n.cfg.Domain, "shop": ev.shop.Name}
	if o := ev.order; o != nil {
		v["orderCode"], v["id"], v["items"], v["slot"] = o.Code, o.ID.String(), fmt.Sprint(o.Items), o.Slot
	}
	if ev.m.Type == enum.EventTypeRefundRequested {
		v["amount"] = money.Decimal(ev.d.AmountCents)
	}
	if template == notify.OrderOut {
		code, err := n.orders.DeliveryCodeOf(ctx, tx, ev.order.ID)
		if apperr.Is(err, apperr.InvalidTransition) {
			return nil, errStale
		} else if err != nil {
			return nil, err
		}
		v["code"] = code
	}
	return v, nil
}
