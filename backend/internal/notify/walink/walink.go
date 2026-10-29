// Package walink is the free WhatsApp channel (docs/09 section 3.2): nothing is
// sent. The service records a LINK row, and the person at the shop taps a
// https://wa.me/ link that opens WhatsApp with the text ready.
package walink

import (
	"context"
	"net/url"
	"strings"

	"gp/internal/notify"
)

type Channel struct{}

func New() *Channel { return &Channel{} }

// URL is the link for a phone in +94 form and a text.
func URL(phone, text string) string {
	return "https://wa.me/" + strings.TrimPrefix(phone, "+") + "?text=" + url.QueryEscape(text)
}

// Send checks that a link can be made. There is no provider, so no reference and no cost.
func (Channel) Send(_ context.Context, m notify.Message) (string, int, error) {
	_, err := notify.TextOf(m)
	return "", 0, err
}
