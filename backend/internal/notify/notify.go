// Package notify sends the platform's texts (docs/07 section 13, docs/09 section
// 3): it renders a template, picks the channel, keeps the daily cap and writes one
// row in `notifications` for every try. The channels (SMS gateway, wa.me link, log)
// are small packages of their own; this one knows none of their formats.
package notify

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Message is one thing to send. Template names a text in templates.go; Vars fill it.
type Message struct {
	To       string // phone number in +94 form
	Template string
	Vars     map[string]string
	// Channel defaults to SMS.
	Channel enum.NotifyChannel
	ShopID  *uuid.UUID
	UserID  *uuid.UUID
	EventID string // the outbox event that caused it, for de-duplication
	// Text is the rendered text. The service fills it before a channel sees the
	// message; a channel never renders on its own.
	Text string
}

// Channel delivers a message and returns the provider's reference and cost.
type Channel interface {
	Send(ctx context.Context, m Message) (ref string, cost int, err error)
}

// Sender is what other modules depend on.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// ErrCapReached says the day's SMS cap is used up. The message was recorded as
// skipped and not sent; callers that can wait (events) go on, a person who waits
// for a code (OTP) is told.
var ErrCapReached = errors.New("notify: the daily SMS cap is reached")

// Deps is everything the service is built from.
type Deps struct {
	DB *db.DB
	// SMS and WALink are the channels. A nil channel means that channel is not
	// available and its messages are recorded as skipped.
	SMS, WALink Channel
	// DailyCap is the most SMS sent in one Colombo day, all kinds together.
	DailyCap int
	// AdminPhone gets the cap alerts. App is the platform name in them.
	AdminPhone, App string
}

type Service struct {
	db         *db.DB
	channels   map[enum.NotifyChannel]Channel
	dailyCap   int
	adminPhone string
	app        string
}

func NewService(d Deps) *Service {
	ch := map[enum.NotifyChannel]Channel{}
	if d.SMS != nil {
		ch[enum.NotifyChannelSms] = d.SMS
	}
	if d.WALink != nil {
		ch[enum.NotifyChannelWaLink] = d.WALink
	}
	return &Service{db: d.DB, channels: ch, dailyCap: d.DailyCap, adminPhone: d.AdminPhone, app: d.App}
}

// Send checks the message, applies the rules and hands it to the channel. A
// message with an unknown template or a missing variable is a bug in the caller,
// so it fails here and never reaches a customer half written. The result is
// always written to `notifications`, with the phone masked and no text.
//
// A message that was already sent for the same event, template and recipient is
// not sent again, so a retried event does not text anyone twice.
func (s *Service) Send(ctx context.Context, m Message) error {
	if m.To == "" {
		return fmt.Errorf("notify: no recipient for template %q", m.Template)
	}
	if m.Channel == "" {
		m.Channel = enum.NotifyChannelSms
	}
	text, err := Render(m.Template, m.Vars)
	if err != nil {
		return err
	}
	m.Text = text
	ch, ok := s.channels[m.Channel]
	if !ok {
		return s.record(ctx, m, outcome{status: statusSkipped, reason: "no_channel"})
	}
	if m.EventID != "" {
		if done, err := s.alreadySent(ctx, m); err != nil || done {
			return err
		}
	}
	if m.Channel == enum.NotifyChannelSms {
		if err := s.checkCap(ctx, m); err != nil {
			return err
		}
	}
	ref, cost, sendErr := ch.Send(ctx, m)
	if sendErr != nil {
		if err := s.record(ctx, m, outcome{status: statusFailed, reason: sendErr.Error()}); err != nil {
			return err
		}
		return sendErr
	}
	status := statusSent
	if m.Channel == enum.NotifyChannelWaLink {
		status = statusLink
	}
	if err := s.record(ctx, m, outcome{status: status, ref: ref, cost: cost}); err != nil {
		return err
	}
	if m.Channel == enum.NotifyChannelSms {
		s.warnAtEighty(ctx)
	}
	return nil
}

// TextOf is the text a channel sends: the one the service rendered, or, for a
// channel used on its own, the template rendered now.
func TextOf(m Message) (string, error) {
	if m.Text != "" {
		return m.Text, nil
	}
	return Render(m.Template, m.Vars)
}
