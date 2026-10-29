package logchannel

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"gp/internal/notify"
	"gp/internal/shared/logx"
)

func send(t *testing.T, showText bool) string {
	t.Helper()
	var buf bytes.Buffer
	ctx := logx.With(context.Background(), logx.New(slog.LevelInfo, &buf))
	m := notify.Message{To: "+94771234567", Template: notify.OTP, Vars: map[string]string{"app": "NearShop", "code": "482913"}}
	if _, _, err := New(showText).Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestLogChannelMasksByDefault(t *testing.T) {
	out := send(t, false)
	if !strings.Contains(out, "notify.log_channel") || strings.Contains(out, "482913") || strings.Contains(out, "771234567") {
		t.Errorf("log line leaks or is missing: %s", out)
	}
}

func TestLogChannelLocalShowsCodeButNotNumber(t *testing.T) {
	out := send(t, true)
	if !strings.Contains(out, "482913") {
		t.Errorf("local mode must show the code: %s", out)
	}
	if strings.Contains(out, "771234567") {
		t.Errorf("the phone number must stay masked: %s", out)
	}
}
