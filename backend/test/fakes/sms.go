package fakes

import (
	"context"
	"sync"

	"gp/internal/notify"
	"gp/internal/shared/apperr"
)

// PassToken is the Turnstile token the fake accepts; any other token is refused.
const PassToken = "turnstile-pass"

// Outbox is a notify channel that keeps what would have been sent, so a test can
// read the OTP code the way the customer would from the SMS.
type Outbox struct {
	mu   sync.Mutex
	sent []notify.Message
}

func (o *Outbox) Send(_ context.Context, m notify.Message) (string, int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, m)
	return "fake", 0, nil
}

// Sent is a copy of all messages so far, oldest first.
func (o *Outbox) Sent() []notify.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]notify.Message(nil), o.sent...)
}

// LastCode is the "code" variable of the newest message to phone, or "".
func (o *Outbox) LastCode(phone string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i := len(o.sent) - 1; i >= 0; i-- {
		if o.sent[i].To == phone {
			return o.sent[i].Vars["code"]
		}
	}
	return ""
}

// Turnstile accepts PassToken and refuses everything else.
type Turnstile struct{}

func (Turnstile) Verify(_ context.Context, token, _ string) error {
	if token != PassToken {
		return apperr.New(apperr.BotCheckFailed)
	}
	return nil
}
