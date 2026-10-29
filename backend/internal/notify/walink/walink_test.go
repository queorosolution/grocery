package walink

import (
	"context"
	"testing"

	"gp/internal/notify"
)

func TestURL(t *testing.T) {
	got := URL("+94771234567", "NearShop: order KS-1 is on the way & more?")
	want := "https://wa.me/94771234567?text=NearShop%3A+order+KS-1+is+on+the+way+%26+more%3F"
	if got != want {
		t.Errorf("URL = %s\nwant  %s", got, want)
	}
}

func TestSend_NeedsARenderableMessage(t *testing.T) {
	if _, _, err := New().Send(context.Background(), notify.Message{To: "+94771234567", Template: "nope"}); err == nil {
		t.Error("an unknown template must fail")
	}
	m := notify.Message{To: "+94771234567", Template: notify.OrderExpired, Vars: map[string]string{"app": "A", "orderCode": "C-1"}}
	if ref, cost, err := New().Send(context.Background(), m); err != nil || ref != "" || cost != 0 {
		t.Errorf("send = %q, %d, %v", ref, cost, err)
	}
}
