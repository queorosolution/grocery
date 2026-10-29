// Package logchannel is the channel for local and dev: it writes to the log and
// sends nothing. Like every other log line, phone numbers and codes are masked.
package logchannel

import (
	"context"
	"log/slog"

	"gp/internal/notify"
	"gp/internal/shared/logx"
)

type Channel struct {
	// ShowText also logs the rendered text, which holds the OTP. Only the local
	// environment sets it, so a developer can sign in without a real SMS.
	showText bool
}

func New(showText bool) *Channel { return &Channel{showText: showText} }

func (c *Channel) Send(ctx context.Context, m notify.Message) (string, int, error) {
	attrs := []any{"template", m.Template, "to", m.To}
	if c.showText {
		text, err := notify.TextOf(m)
		if err != nil {
			return "", 0, err
		}
		attrs = append(attrs, "text", text)
	}
	logx.From(ctx).LogAttrs(ctx, slog.LevelInfo, "notify.log_channel", slog.Group("msg", attrs...))
	return "log", 0, nil
}
