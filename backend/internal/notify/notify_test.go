package notify

import (
	"context"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	got, err := Render(OTP, map[string]string{"platform": "NearShop", "code": "123456"})
	if err != nil || got != "NearShop: 123456 is your verification code. It works for 10 minutes. Do not share it." {
		t.Fatalf("render = %q, %v", got, err)
	}
	if _, err := Render("nope", nil); err == nil {
		t.Error("unknown template must fail")
	}
	if _, err := Render(OTP, map[string]string{"platform": "NearShop"}); err == nil {
		t.Error("a missing value must fail, not send {code}")
	}
}

func TestRenderCleansValues(t *testing.T) {
	got, err := Render(OTP, map[string]string{"platform": "Evil\r\n{code}" + strings.Repeat("x", 100), "code": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(got, "\r\n") || strings.Count(got, "{") != 0 || len([]rune(got)) > maxSMSLen {
		t.Errorf("value was not cleaned: %q", got)
	}
}

// Every template fits one SMS even when its values are as long as allowed.
func TestTemplatesFit(t *testing.T) {
	for name, text := range templates {
		vars := map[string]string{}
		for _, slot := range slots(text) {
			vars[slot] = strings.Repeat("W", maxVarLen)
		}
		if _, err := Render(name, vars); err != nil {
			t.Errorf("template %q: %v", name, err)
		}
	}
}

func slots(text string) []string {
	var out []string
	for {
		i := strings.IndexByte(text, '{')
		if i < 0 {
			return out
		}
		j := strings.IndexByte(text[i:], '}')
		out = append(out, text[i+1:i+j])
		text = text[i+j+1:]
	}
}

type recorder struct{ sent []Message }

func (r *recorder) Send(_ context.Context, m Message) (string, int, error) {
	r.sent = append(r.sent, m)
	return "ref", 0, nil
}

func TestServiceSendChecksFirst(t *testing.T) {
	rec := &recorder{}
	s := New(rec)
	ok := Message{To: "+94771234567", Template: OTP, Vars: map[string]string{"platform": "P", "code": "1"}}
	if err := s.Send(context.Background(), ok); err != nil || len(rec.sent) != 1 {
		t.Fatalf("send: %v, %d sent", err, len(rec.sent))
	}
	for name, m := range map[string]Message{
		"no recipient":     {Template: OTP, Vars: ok.Vars},
		"unknown template": {To: ok.To, Template: "x"},
		"missing var":      {To: ok.To, Template: OTP},
	} {
		if err := s.Send(context.Background(), m); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if len(rec.sent) != 1 {
		t.Errorf("a refused message reached the channel (%d sent)", len(rec.sent))
	}
}
