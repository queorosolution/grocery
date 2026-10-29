// Package notify sends messages to customers, shops and drivers. This first
// version has the channel interface, the templates and the log channel; the real
// SMS, WhatsApp and wa.me channels come with P25.
package notify

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Message is one thing to send. Template names a text in templates.go; Vars fill it.
type Message struct {
	To       string // phone number in +94 form
	Template string
	Vars     map[string]string
	ShopID   *uuid.UUID
	UserID   *uuid.UUID
	EventID  string // the outbox event that caused it, for de-duplication
}

// Channel delivers a message and returns the provider's reference and cost.
type Channel interface {
	Send(ctx context.Context, m Message) (ref string, cost int, err error)
}

// Service picks the channel. Today there is one.
type Service struct{ channel Channel }

func New(c Channel) *Service { return &Service{channel: c} }

// Send checks the message and hands it to the channel. A message with an unknown
// template or a missing variable is a bug in the caller, so it fails here and
// never reaches a customer half written.
func (s *Service) Send(ctx context.Context, m Message) error {
	if m.To == "" {
		return fmt.Errorf("notify: no recipient for template %q", m.Template)
	}
	if _, err := Render(m.Template, m.Vars); err != nil {
		return err
	}
	_, _, err := s.channel.Send(ctx, m)
	return err
}

// Sender is what other modules depend on.
type Sender interface {
	Send(ctx context.Context, m Message) error
}
