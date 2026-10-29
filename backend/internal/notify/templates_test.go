package notify

import (
	"strings"
	"testing"
)

const (
	someID = "6f1c1f0e-8a52-4d8a-9c1d-0123456789ab"
)

func TestRender(t *testing.T) {
	got, err := Render(OTP, map[string]string{"app": "NearShop", "code": "123456"})
	if err != nil || got != "NearShop: your code is 123456. Valid 10 min. Do not share it." {
		t.Fatalf("render = %q, %v", got, err)
	}
	if _, err := Render("nope", nil); err == nil {
		t.Error("unknown template must fail")
	}
	if _, err := Render(OTP, map[string]string{"app": "NearShop"}); err == nil {
		t.Error("a missing value must fail, not send {code}")
	}
	got, err = Render(OrderApproved, map[string]string{"app": "NearShop", "shop": "Krishna Stores", "orderCode": "KS-7Q2M9X", "domain": "nearshop.lk", "id": someID})
	want := "NearShop: Krishna Stores confirmed order KS-7Q2M9X. Pay here: https://www.nearshop.lk/order/" + someID
	if err != nil || got != want {
		t.Fatalf("approved = %q, %v", got, err)
	}
}

// HDR-01: control characters, braces and long values in names never reach the text.
func TestRender_HDR01_CleansValues(t *testing.T) {
	got, err := Render(OrderRejected, map[string]string{"app": "NearShop", "orderCode": "KS-1", "shop": "Evil\r\nShop {code}\x00" + strings.Repeat("x", 100)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(got, "\r\n\x00") || strings.Count(got, "{") != 0 || len([]rune(got)) > maxSMSLen {
		t.Errorf("value was not cleaned: %q", got)
	}
	if !strings.Contains(got, "EvilShop code") {
		t.Errorf("the name should stay readable without the bad characters: %q", got)
	}
}

// A name cannot add a link or a slot: only the two link slots hold a link, and they are checked.
func TestRender_LinkSlotsAreChecked(t *testing.T) {
	ok := map[string]string{"app": "A", "shop": "S", "orderCode": "C-1", "domain": "nearshop.lk", "id": someID}
	for name, change := range map[string]map[string]string{
		"domain with a path":    {"domain": "evil.lk/phish?x="},
		"domain with a space":   {"domain": "nearshop.lk evil.lk"},
		"domain with user info": {"domain": "a@evil.lk"},
		"domain too long":       {"domain": strings.Repeat("a", 21) + ".lk"},
		"id that is not an id":  {"id": "../../evil"},
		"id with a newline":     {"id": someID + "\n"},
	} {
		vars := map[string]string{}
		for k, v := range ok {
			vars[k] = v
		}
		for k, v := range change {
			vars[k] = v
		}
		if _, err := Render(OrderApproved, vars); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	// Names are only text: a link typed into one loses its scheme and its path.
	got, err := Render(OrderRejected, map[string]string{"app": "A", "orderCode": "C-1", "shop": "https://evil.lk/x me@evil.lk"})
	if err != nil || strings.Contains(got, "://") || strings.ContainsAny(got, "/@") {
		t.Errorf("a name still holds link characters: %q (%v)", got, err)
	}
}

// docs/09 section 3.1: every text fits one SMS segment, with every slot at its limit.
func TestTemplates_Max160(t *testing.T) {
	for name, text := range templates {
		vars := map[string]string{}
		for _, slot := range slotsOf(text) {
			limit, ok := slotLimits[slot]
			if !ok {
				t.Fatalf("template %q: the slot %q has no limit", name, slot)
			}
			vars[slot] = strings.Repeat("W", limit)
		}
		vars["domain"] = strings.Repeat("a", slotLimits["domain"])
		vars["id"] = someID
		got, err := Render(name, vars)
		if err != nil {
			t.Errorf("template %q: %v", name, err)
			continue
		}
		if n := len([]rune(got)); n > maxSMSLen {
			t.Errorf("template %q is %d characters", name, n)
		}
	}
}

// docs/09: the only link a text holds is https://www.<domain>/…
func TestTemplates_OnlyPlatformLinks(t *testing.T) {
	for name, text := range templates {
		rest := text
		for {
			i := strings.Index(rest, "http")
			if i < 0 {
				break
			}
			if !strings.HasPrefix(rest[i:], "https://www.{domain}/") {
				t.Errorf("template %q has a link that is not the platform's", name)
			}
			rest = rest[i+4:]
		}
	}
}

func TestTemplates_AllNamesExist(t *testing.T) {
	for _, name := range []string{OTP, ShopNewBill, OrderApproved, OrderRevised, OrderRejected, OrderPaid, OrderOut, OrderDelivered,
		OrderExpired, OrderCancelled, OrderDeliveryFailed, OrderRefundCancel, AdminRefund, AdminConnector, AdminCap80, AdminCap100} {
		if _, ok := templates[name]; !ok {
			t.Errorf("no template %q", name)
		}
	}
}
